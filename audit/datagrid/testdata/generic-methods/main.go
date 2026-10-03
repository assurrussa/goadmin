package main

type Builder struct{}

type Typed[T any] struct{ Value T }

// A concrete method declaring its own type parameter, independent of receiver.
func (Builder) With[T any](value T) Typed[T] { return Typed[T]{Value: value} }

func main() {
	b := Builder{}
	if b.With(42).Value != 42 || b.With("typed").Value != "typed" {
		panic("generic method mismatch")
	}
	println("Go 1.27 concrete generic method: PASS")
}
