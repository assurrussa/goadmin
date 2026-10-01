package pointer

func To[T any](value T) *T { return &value }
func Indirect[T any](value *T) (result T) {
	if value != nil {
		return *value
	}
	return result
}
