package shared

func MustRet[T any](item T, err error) T {
	if err != nil {
		panic(err)
	}
	return item
}
