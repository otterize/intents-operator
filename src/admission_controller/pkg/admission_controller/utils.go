package admission_controller

func ObjAsPtr[T any](obj T) *T {
	return &obj
}
