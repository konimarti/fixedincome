# Changelog

## WP4: Monte Carlo (breaking)

`pkg/mc` has been rewritten: a model is discretised on an explicit time
grid, paths are simulated exactly (no Euler bias), the engine is parallel,
seedable and allocation-free per path, and the estimates come with honest
standard errors.

### Design

- `mc.Grid` has N+1 nodes `t_0 = 0, ..., t_N = T` (`NewUniformGrid(T, n)`,
  `NewGrid(times)`); previously the paths had N points and stopped at
  `T - dt`.
- `mc.Model.Discretize(grid) (Simulator, error)` precomputes the grid
  coefficients; `Simulator.Simulate(z, path)` maps `Dim()` standard
  normals to a `mc.Path{Grid, State [][]float64, Df []float64}` with the
  pathwise discount factor `Df[i] = exp(-∫_0^{t_i} r ds)`. Negating `z`
  gives the antithetic path.
- `mc.Price(model, payoff, grid, opts)`, `mc.PriceMulti` (vector payoffs,
  e.g. `mc.ZeroBonds` for the whole model curve) and `mc.PriceAmerican`
  (Longstaff-Schwartz) return `mc.Result{Mean, StdErr, N, Paths, Beta}`
  with `CI95()`.
- `mc.Options{Paths, Workers, Seed, Antithetic, Control, TrainingPaths}`:
  the samples are split into blocks of `mc.BlockSize` with one xoshiro256**
  stream per block derived from `(Seed, block)`; results are identical for
  any number of workers (default `GOMAXPROCS`). Welford/Chan accumulation,
  variance with n-1. The block statistics are merged in block order as
  soon as all earlier blocks are done (reorder window of 2×Workers
  blocks), so the memory does not grow with the number of paths
  (`PriceMulti` with `ZeroBonds`, N = 1000: 1.8 / 15.7 / 58.7 MB for
  2^14 / 2^18 / 2^20 paths before, 1.8 / 4.6 / 4.5 MB now).
- Control variates: β is estimated on a pilot sample (the first
  min(256, samples/10) samples, excluded from the estimate), so the
  estimator is unbiased and its standard error honest for small samples;
  at least `mc.MinControlSamples` (30) samples are required.
- Panics in simulators, payoffs, controls and exercise functions are
  returned as errors (`mc.ErrPanic`); `PriceMulti` rejects a `ZeroBonds`
  on another grid.
- Short rate models (`holee`, `hullwhite`, `vasicek`) share an exact
  simulation of `(x, ∫x)` for the OU factor (2 normals per step); the
  deterministic part of `∫r` is computed exactly on the nodes, so
  `E[Df[i]]` equals the model bond price for every grid (Ho-Lee and
  Hull-White: the curve; σ = 0: to machine precision).
- `stock.GBM` simulates `ln S` exactly with the forward rates of a curve
  (`Df[i] S_i` is a martingale on the grid) and a dividend yield `Q`.

### Migration

| Old API | New API |
|---|---|
| `mc.New(model, nsim)`, `e.Run()`, `e.Estimate()`, `e.StdError()`, `e.CI()` | `res, err := mc.Price(model, payoff, grid, mc.Options{Paths: nsim, Seed: s})`; `res.Mean`, `res.StdErr` (n-1), `res.CI95()` |
| `mc.Model{Measurement() float64}` | `mc.Model{Discretize(*Grid) (Simulator, error)}` + `mc.Payoff{Value(*Path) float64}` (`mc.PayoffFunc`) |
| `Engine.Estimates` (all samples stored) | not stored (streaming moments, memory independent of `Paths`) |
| `model.Rng = rand.New(...)` (time-seeded by default) | `Options.Seed` (deterministic, worker-independent); `mc.NewRand(seed, stream)` |
| payoff `func(rates []float64) float64` with N points `0..(N-1)dt`, discounting by summing `rates[i]*dt` | `func(p *mc.Path) float64` on N+1 nodes `0..T`, discount with `p.Df[i]` |
| `holee.New(ts, sigma, T, N, payoff)`; fields `R0, Theta, Rng, Payoff`; `holee.Calibrate` | `holee.New(curve, sigma) (*HoLee, error)` (calibration implicit and exact); `R0()`, `Theta(t)`, `Phi(t)`, `Z(r, t, T)`, `Discretize(grid)` |
| — | `hullwhite.New(curve, a, sigma)`: arbitrage-free Hull-White with `Z(r, t, T)`, `Theta`, `Phi`, `IntPhi` |
| `vasicek.New(ts, sigma, T, N, payoff)` (calibrates r̄, γ silently) | `vasicek.New(r0, rbar, gamma, sigma)`; `vasicek.Calibrate(curve, sigma, maturities) (*Vasicek, Calibration{RMSE, Evaluations, AtBound}, error)` (R0 from the curve) |
| `vasicek.Calibrate(v, ts)` | `vasicek.Calibrate(curve, sigma, maturities)` |
| `v.Z(r, t, T)` (singular for γ -> 0) | `v.Z(r, t, T)` (γ >= 0, no singularity), `v.ZeroRate(T)`, `v.Mean(t)` |
| Vasicek Euler scheme | exact OU transition and exact joint `∫r` |
| `stock.New(r, s0, sigma, T, N, payoff)` (R: "return") | `stock.New(s0, sigma, q, curve)`; the drift is the risk-free forward rate of the curve minus the dividend yield `q` |
| payoff closures for options | `stock.European{Type, Strike}`, `stock.Asian{Type, Strike, Geometric}`, `stock.American{Type, Strike, Degree, Model}` (`mc.Exercisable`; use `m.American(type, K)`) |
| — | `m.European(type, K, T)` (Black-Scholes), `m.GeometricAsian(type, K, grid)` (closed form), `m.TerminalControl(grid)`, `m.EuropeanControl(type, K, grid)`, `m.GeometricAsianControl(type, K, grid)` |
| — | `mc.PriceAmerican(model, exercisable, grid, opts)` → `AmericanResult{Result (out-of-sample), InSample, InSampleStdErr, TrainingPaths}`; optional `mc.ContinuationBounder`, `mc.Validator` |
| `cmds/holee` writes `./result.csv` without header (committed sample `cmds/holee/result.csv`, removed); the Monte Carlo part integrated the rates only up to `T - dt` | `holee -f term.json -n 1000 -m 10 -s 0.02 -paths 10000 -seed 0 -o holee.csv` (CSV with header to stdout or `-o`; summary and warnings on stderr); `Rscript analysis.R holee.csv holee.png` |

### Intentional behaviour changes

- Ho-Lee: the old calibration used forwards lagged by one step and
  integrated the rates up to `T - dt`; the zero bond price was biased by
  O(dt) even for σ = 0. Exact expectation of the old scheme (NSS curve of
  the tests): T = 30, N = 50, σ = 0.02: 100.48 instead of 106.09 (-5.61);
  T = 10, N = 1000, σ = 0.02: -0.030; σ = 0, T = 30, N = 50: -0.37. New:
  exact for σ = 0 (error < 1e-12), unbiased for σ > 0 (2^20 antithetic
  paths: 106.131 ± 0.592 vs 106.088).
- Vasicek: the old calibration (NSS curve of the tests, σ = 0, T = 5,
  N = 1260, fit up to 1.5 T) ran away to r̄ = 122.5 with γ ≈ 0 (singular
  parametrisation; σ = 0.01: r̄ = 16.05). Now (κ = γ r̄, bounded γ) with
  the RMSE reported; Hull-White is the arbitrage-free alternative.
- American options (Longstaff-Schwartz): the exercise rule only exercises
  if the intrinsic value is at least the regression estimate and at least
  a known lower bound of the continuation value (`mc.ContinuationBounder`;
  for `stock.American` with a model: the European value of the remaining
  option, which is also a basis function). Without it a call without
  dividends was sometimes exercised early (LSM below the European price by
  more than 4 SE for 3 of 60 seeds); now it equals the European Monte
  Carlo price exactly. Default basis degree 3.
- Units: all model parameters are decimals; the initial short rate is set
  from the curve (`f(0,0)`), not passed in.
- `example/range_floater` reports four variants (flat or model LIBOR ×
  LIBOR roll-up or pathwise discounting); 200000 paths, seed 1: flat+roll
  (Veronesi) 99.8116, flat+Df 99.5759, model+roll 99.6022, model+Df
  (consistent) 99.6070 (book 99.8385 ± 0.0505). The gap comes from
  discounting at the flat-converted LIBOR, see the example.

## Unreleased: core redesign (breaking)

The core of the library (curves, schedules, cash flows, instrument
valuation) has been redesigned. The API is not backwards compatible.

### Conventions

- All rates, yields, spreads, betas, volatilities and dividend yields are
  **decimals** inside the library (`0.02` = 2%, `0.005` = 50bp). Percent and
  basis points are only used at the I/O boundary (curve files, command line
  flags).
- Rates are **continuously compounded** everywhere. `pkg/rate` converts to and
  from n-times compounded and simple rates (`cashflow.Yield` reports the
  annually compounded yield).
- The time axis for discounting is **ACT/365 Fixed** (calendar days / 365 from
  the settlement date) for all instruments, independent of the day count
  convention of accrued interest. Dates are normalised to UTC midnight.
- Curves are immutable and safe for concurrent use. Spreads are applied with
  `term.WithSpread`, which returns a new curve.
- Every fixed-income instrument reduces to cash flows (`cashflow.CashFlow`);
  analytics operate on cash flows.
- Invalid user input yields errors instead of panics.

### Migration

| Old API | New API |
|---|---|
| `term.Structure` | `term.Curve` (`Discount(t)`, `ZeroRate(t)`) |
| `ts.Z(t)` | `c.Discount(t)` |
| `ts.Rate(t)` (cc, percent) | `c.ZeroRate(t)` (cc, decimal) |
| `ts.SetSpread(bp)` / `GetSpread()` | `term.WithSpread(c, bp/1e4)` / `term.Spread(c)` |
| `term.Flat{R: 2.0, Spread: 10}` | `term.WithSpread(term.Flat{Rate: 0.02}, 0.001)` |
| `term.NelsonSiegelSvensson{B0..B3 (%), T1, T2, Spread}` | `term.NewNSS(b0..b3 (decimal), tau1, tau2)` or `term.NSS{B0, B1, B2, B3, Tau1, Tau2}` + `Validate()` |
| `term.NewSpline(t, z, spread)` (panics; cubic spline on DFs) | `term.NewSpline(t, z) (*Spline, error)` (natural cubic spline on log-DFs, anchored at (0,1)) |
| `term.Spline{...}.Init()`, `term.Initer` | removed; use `term.NewSpline` |
| — | `term.Forward(c, t1, t2)`, `term.InstForward(c, t)` |
| `term.Parse(data) (Structure, error)` | `term.Parse(data) (Curve, Meta, error)`; new schema with `"model"` (case-insensitive), optional `"version": 1`, `"comment"` and `"x-..."` extension keys; legacy format still parsed |
| `json.Marshal(ts)` | `term.Marshal(c, meta)` |
| `rate.Continuous(i%, n)` | `rate.ToContinuous(i, n) (float64, error)` |
| `rate.Annual(r%, n)` | `rate.FromContinuous(rc, n) (float64, error)` |
| `rate.EffectiveAnnual(i%, n)` (growth factor) | `rate.EffectiveAnnual(i, n)` (effective rate, `(1+i/n)^n - 1`) |
| — | `rate.ToSimple`, `rate.FromSimple` |
| `maturity.Schedule{Settlement, Maturity, Frequency int, Basis string}` | `maturity.Schedule{Issue, FirstCoupon, Maturity, Frequency maturity.Frequency, DayCount maturity.DayCount, EOM maturity.EOMRule}`; the settlement date is passed to the methods |
| `Basis: "30E360"` (string) | `DayCount: maturity.Thirty360E` (`Eurobond`, `BondBasis`, `Act360`, `ActAct`); `maturity.ParseDayCount(name)` accepts the daycount names and aliases such as `"30E/360"`, `"ACT/360"` |
| — | `maturity.ParseFrequency`, `maturity.DayCountNames`, `maturity.EOMAuto/EOMOn/EOMOff` |
| `Frequency: 0` (= annual) | `maturity.Annual` (0 is invalid, see `Frequency.Validate`) |
| `s.M()` (descending, basis-dependent years) | `s.CouponDates(settle) ([]time.Time, error)` (ascending dates), `s.Periods(settle) ([]Period, error)`, `s.PeriodAt(settle) (Period, error)`, `maturity.YearFraction(settle, d)` |
| `s.Last()`, `s.Next()` | `maturity.YearFraction(settle, s.Maturity)`, `s.NextCoupon(settle) (time.Time, error)` (`maturity.ErrMatured`) |
| `s.DayCountFraction()` | `p, _ := s.PeriodAt(settle); s.AccrualFraction(p.Start, settle, p.End)` (year fraction, with error) |
| `s.Compounding()`, `s.EffectiveCoupon(c)` | `int(s.Frequency)`, `s.CouponFraction(period) (float64, error)` |
| `maturity.DaysInYear`, `LastDay`, `FirstDay`, `DifferenceInYears` | removed (`maturity.YearFraction`, `maturity.Date`) |
| `fixedincome.Security`, `TermSecurity`, `Option` | `cashflow.Instrument` (`CashFlows(settle)`: Straight) and `cashflow.Projected` (`CashFlows(settle, forecast)`: Floating, InterestRateSwap, FRA) |
| `b.PresentValue(ts)` (bond) | `b.PresentValue(settle, c)` = `b.DirtyPrice(settle, c)`, or `cashflow.PV(cfs, c)` |
| `b.PresentValue(ts) - b.Accrued()` | `b.CleanPrice(settle, c)` |
| `b.Accrued()` | `b.Accrued(settle) (float64, error)` |
| `b.Duration(ts)` (negative) | `cashflow.Duration(cfs, c)` (**positive** modified duration) |
| `b.Convexity(ts)` | `cashflow.Convexity(cfs, c)` |
| `fixedincome.Irr(dirty, s)` (cc, percent) | `cashflow.YieldCC(dirty, cfs)` (cc, decimal); `cashflow.Yield(dirty, cfs, n)` |
| `fixedincome.Spread(dirty, s, ts)` (bp, mutates ts) | `cashflow.ZSpread(dirty, cfs, c)` (decimal, no mutation) |
| `fixedincome.PVBP(s, ts)` | `cashflow.DV01(cfs, c)` (full revaluation, negative for a long bond) |
| `fixedincome.InterestSensitivity(dr, s, ts)` | `cashflow.Sensitivity(cfs, c, dr)` (`-D dr + 1/2 C dr²`) |
| — | `cashflow.KeyRateDurations(cfs, c, keys, bump)` |
| — | sentinel errors `cashflow.ErrNoYield`, `cashflow.ErrNoCashFlows`, `maturity.ErrMatured`, `maturity.ErrSettlement`, `bond.ErrNoFixing`, `swap.ErrNoFixing` |
| — | `term.Parametric` (NSS: `Params`, `WithParams`, `ZeroRateGrad`, `DiscountGrad`), `term.DiscountGrid`, `term.MinKnotSpacing` |
| `bond.Straight{Schedule, Coupon (%), Redemption}` | `bond.NewStraight(schedule, coupon (decimal), bond.WithFace(f), bond.WithRedemption(r))` (Face 100 and Redemption = Face by default, only via the constructor); struct literals must set `Face > 0` and `Redemption >= 0` explicitly |
| `bond.Floating{Schedule, Rate (%), Redemption}`; `PresentValue(ts)` | `bond.NewFloating(schedule, margin, bond.WithFixing(r), ...)` / `bond.Floating{Schedule, Margin, CurrentFixing *float64, Face, Redemption}`; `PresentValue`/`DirtyPrice`/`CleanPrice(settle, discount, forecast)`, `CashFlows(settle, forecast)`, `Accrued(settle)` |
| `f.Duration(ts)`, `f.Convexity(ts)` (FRN) | `bond.FloatingDuration(f, settle, disc, fwd)`, `bond.FloatingConvexity(...)` (both curves shifted) |
| `swap.InterestRateSwap{Floating, Fixed}` (bond legs) | `swap.InterestRateSwap{Notional (> 0), FixedRate, FixedSchedule, FloatSchedule, Margin, CurrentFixing *float64, Payer}`; `PresentValue`, `CashFlows` (net, signed), `ParRate`, `Annuity`, `FloatLeg` |
| `swap.InterestRate(times, n, ts)` (percent) | `swap.ParRateFromTimes(times, n, c)` (decimal) |
| `swap.FxRate(fx, cfs, times, tsLong, tsShort)` | `swap.FxRate(fx, cfs, times, domestic, foreign)` |
| `forward.RateAgreement{N, M, T1, T2}` | `forward.NewFRA(notional, fixed, start, end, payer)` (Act360) / `forward.FRA{Notional (> 0), FixedRate, Start, End, DayCount maturity.DayCount, Payer}`; `PresentValue(settle, disc, fwd)`, `CashFlows(settle, fwd)`, `ForwardRate`, `AccrualFraction`; ActAct is rejected |
| `forward.ZeroBondPrice(t, m, ts) (float64, error)` | `forward.ZeroBondPrice(t, m, c) float64` |
| — | `forward.FxPrice`, `forward.AssetPrice` |
| `option.European{Type int, S, K, T, Q (%), Vola}` | `option.European{Type option.Type, S, K, T, Vol, Q (decimal)}` |
| `option.Call = 0`, `option.Put = 1` (int) | `option.Call = 1`, `option.Put = 2` (typed `option.Type`; the zero value is invalid); `option.ParseType("call")` |
| `opt.PresentValue(ts)` (panics for invalid type) | `opt.Price(c) (float64, error)` |
| `opt.Delta/Gamma/Rho/Vega(ts)` | `opt.Greeks(c) (Greeks, error)` with `Delta, Gamma, Vega, Theta, Rho, Phi` |
| `fixedincome.ImpliedVola(price, o, ts)` (mutates o) | `option.ImpliedVol(price, opt, c)` (no mutation, bracket [1e-6, 5]; `option.ErrNoImpliedVol`, `option.ErrNoVega`) |
| `option.D1(S, K, T, Q%, vol, ts)` | `option.D1(S, K, T, r, q, vol)` |
| `holee.New(ts, ...)`, `vasicek.New(ts, ...)` | redesigned in WP4, see "WP4: Monte Carlo" below |

### Intentional behaviour changes

- **Time axis**: discounting times are ACT/365F for all day count conventions
  (previously the day count convention of the bond, e.g. 30E/360 years). For
  the README bond (1.25% CH0224396983, settle 2021-04-17) the dirty price
  changes by +0.0003, the yield by +0.00015% and the z-spread by +0.005bp; for
  CH0193265995 (settle 2021-04-15) the z-spread changes from 24.82bp to 24.90bp.
- **Accrued interest**: for the 30/360 family and ACT/360 it is
  `coupon × daycount.Days(prev, settle)/360` (previously
  `days/periodDays/frequency`, which is wrong for EUROBOND/BONDBASIS periods
  ending at the end of February and for irregular ACT/360 periods). ACT/ACT
  ("ACTACT") is ACT/ACT ICMA (`coupon/freq × days/periodDays`), with the
  notional period for a short first coupon. No change for regular 30E360
  periods (README bond: identical).
- **Duration sign**: modified duration is positive (`-1/P dP/dy`).
- **Spline**: natural cubic spline on the log-discount factors, anchored at
  (0, 1), with flat instantaneous forward extrapolation beyond the last knot
  (previously a cubic spline on the discount factors with flat zero-rate
  extrapolation below the first knot). On the same knots as the termfit
  sample fit the zero rates differ by at most 1bp up to 23y.
- **Settlement on a coupon date**: the coupon is not received (cash flows
  strictly after the settlement date) and nothing is accrued; a bond settled
  on its maturity date has no cash flows and a value of 0.
- **FRN / swap**: coupons are projected from a forecast curve and discounted
  on the discount curve (previously "par at reset" was assumed, even with a
  z-spread). `CurrentFixing` (now `*float64`) is used for the period
  containing the settlement date, including a period starting on it; if it
  is nil, a period starting on the settlement date is projected (so the FRN
  is at par with no rate duration) and a period in progress is an error
  (`ErrNoFixing`). The index is an IBOR-style term rate; compounded-in-arrears
  overnight rates (SARON, SOFR, €STR) are not modelled.
- **ACT/360 bonds**: the coupon is `coupon × days/360` of the period (equal
  to the accrual, no clean price jump on coupon dates; a 5% semiannual bond
  with a 182-day period jumped by 0.0092 before). 30/360 and ACT/ACT ICMA
  regular coupons stay `coupon/frequency`.
- **Long first coupon**: `Schedule.FirstCoupon` (with `Issue`) defines a
  short or long first period; ACT/ACT ICMA accrual runs over the notional
  periods.
- **Yield / z-spread**: a non-positive price for non-negative cash flows (or
  vice versa) returns `ErrNoYield` instead of a spurious root (e.g.
  `YieldCC(0, {30y: 100})` returned 50).
- **Yield(n)** is the n-compounded equivalent of the continuously compounded
  ACT/365F yield, not the ICMA street yield.
- **Validation**: invalid schedules, zero settlement dates (or more than 200
  years before maturity), non-positive face values or notionals, negative
  redemptions and invalid NSS parameters return errors (or NaN for curve
  evaluation) instead of panics or silent defaults. Zero-value and nil
  splines return NaN; spline knots must be at least 1e-8 years apart.
- **EOM rule**: inferred if the maturity is the last day of its month,
  including 28 February in a non-leap year (use `maturity.EOMOff` to keep
  the 28th).
- **Root finding**: own Brent solver (`internal/solve`); roots at bracket
  endpoints are returned, NaN and non-bracketing intervals are skipped. Yields
  use Newton's method with a bracketed fallback up to very high yields.
- **Implied volatility** is searched in [1e-6, 5] (previously [1e-6, 1000]).

### Upstream github.com/konimarti/daycount (not patched here)

- 30E360: the last day of February is converted to 30 even if it is the
  termination date (FIXME).
- No ACT/365F or ACT/ACT ISDA conventions.
- `Implemented()` returns the names in random order.
- `Fraction()` is wrong for EUROBOND/BONDBASIS February-end periods and uses
  a 30E/360 denominator for ACT360 (not used by this library any more).

### Dependencies

- Removed `github.com/cnkei/gospline` (not goroutine-safe) and
  `github.com/khezen/rootfinding` (returned 0 for roots at bracket endpoints).
