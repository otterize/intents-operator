package shared

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func MustRet[T any](item T, err error) T {
	if err != nil {
		panic(err)
	}
	return item
}
