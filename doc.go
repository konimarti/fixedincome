// Package fixedincome is the root of a library for the valuation of fixed
// income securities with spot-rate term structures and continuous-time
// interest rate models.
//
// The library is organised in the following packages:
//
//   - pkg/term: yield curves (flat, Nelson-Siegel-Svensson, cubic spline),
//     forward rates, z-spread overlay and the JSON curve file format
//   - pkg/rate: conversion between compounding conventions
//   - pkg/maturity: coupon schedules, accrual fractions and the ACT/365F
//     time axis
//   - pkg/cashflow: present value, yield, z-spread, duration, convexity,
//     DV01 and key rate durations of deterministic cash flows
//   - pkg/instrument/...: bonds, swaps, forwards/FRAs and European options
//   - pkg/mc: Monte Carlo engine and interest rate models
//
// Conventions: all rates, yields, spreads and volatilities are decimals
// (0.02 = 2%, 0.0001 = 1bp) and continuously compounded unless stated
// otherwise; percent and basis points are only used at the I/O boundary
// (curve files, command line tools). Times are in years, ACT/365 Fixed.
package fixedincome
