package waitfor

func validScheme(scheme string) bool {
	if len(scheme) == 0 || !asciiLetter(scheme[0]) {
		return false
	}

	for i := 1; i < len(scheme); i++ {
		c := scheme[i]
		if !(asciiLetter(c) || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			return false
		}
	}

	return true
}

func asciiLetter(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
