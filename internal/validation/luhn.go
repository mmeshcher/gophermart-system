// Package validation содержит функции валидации входных данных.
package validation

import "unicode"

// IsValidOrderNumber проверяет корректность номера заказа по алгоритму Луна.
func IsValidOrderNumber(number string) bool {
	if number == "" {
		return false
	}

	sum := 0
	double := false

	for i := len(number) - 1; i >= 0; i-- {
		ch := rune(number[i])
		if !unicode.IsDigit(ch) {
			return false
		}
		digit := int(ch - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}

	return sum%10 == 0
}
