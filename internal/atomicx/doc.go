// Package atomicx contains small atomic building blocks used by bufferpool
// runtime structures.
//
// The package exists to keep atomic counters and cache-line padding decisions
// consistent across Pool, LeaseRegistry, and control-plane scaffolding. It does
// not own domain semantics: callers decide whether a counter represents hits,
// bytes, generations, or lifecycle transitions.
//
// atomicx must stay dependency-free apart from the standard library. It must
// not import the root bufferpool package, allocate in hot paths, or hide memory
// ordering behind broad abstractions.
package atomicx
