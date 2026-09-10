package waitfor

func validScheme(scheme string) bool {
	if len(scheme) == 0 || scheme[0] < 'a' || scheme[0] > 'z' {
		return false
	}

	for _, c := range scheme[1:] {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			return false
		}
	}

	return true
}
