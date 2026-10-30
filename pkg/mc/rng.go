package mc

import "math/rand"

// xoshiro is the xoshiro256** pseudo random number generator by Blackman
// and Vigna, seeded with splitmix64. It implements rand.Source64 and is not
// safe for concurrent use (the engine uses one generator per worker).
type xoshiro struct {
	s [4]uint64
}

// splitmix64 advances x and returns the next splitmix64 output.
func splitmix64(x *uint64) uint64 {
	*x += 0x9e3779b97f4a7c15
	z := *x
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// reseed sets the state for the stream with the given seed and stream
// number. Different (seed, stream) pairs give different, statistically
// independent sequences in practice (not a cryptographic guarantee).
func (x *xoshiro) reseed(seed, stream uint64) {
	sm := seed
	h := splitmix64(&sm)
	sm = h ^ (stream * 0xd1b54a32d192ed03)
	sm ^= splitmix64(&sm)
	for i := range x.s {
		x.s[i] = splitmix64(&sm)
	}
	if x.s[0]|x.s[1]|x.s[2]|x.s[3] == 0 {
		x.s[0] = 1
	}
}

func rotl(x uint64, k uint) uint64 { return (x << k) | (x >> (64 - k)) }

// Uint64 returns the next 64 random bits.
func (x *xoshiro) Uint64() uint64 {
	s := &x.s
	result := rotl(s[1]*5, 7) * 9
	t := s[1] << 17
	s[2] ^= s[0]
	s[3] ^= s[1]
	s[1] ^= s[2]
	s[0] ^= s[3]
	s[2] ^= t
	s[3] = rotl(s[3], 45)
	return result
}

// Int63 returns a non-negative 63-bit integer.
func (x *xoshiro) Int63() int64 { return int64(x.Uint64() >> 1) }

// Seed implements rand.Source (stream 0).
func (x *xoshiro) Seed(seed int64) { x.reseed(uint64(seed), 0) }

// NewRand returns a math/rand generator backed by xoshiro256** for the
// given seed and stream number. The engine uses stream b for block b of
// the simulation; NewRand is exported for reproducing single paths and
// for tests. The generator is not safe for concurrent use.
func NewRand(seed, stream uint64) *rand.Rand {
	src := &xoshiro{}
	src.reseed(seed, stream)
	return rand.New(src)
}
