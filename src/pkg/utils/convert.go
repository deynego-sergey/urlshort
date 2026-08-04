package utils

import (
	"errors"
	"fmt"
	"log"
	"strings"
)

type Converter struct {
	alphabet string
	base     int64
}

func NewConverter(alphabet string) *Converter {
	return &Converter{alphabet: alphabet, base: int64(len(alphabet))}
}

// ConvertToStr -
func (c *Converter) ConvertToStr(n int64) string {
	if c.base < 0 {
		log.Fatal(errors.New("l is less than 1"))
	}
	if n == 0 {
		return string(c.alphabet[0])
	}
	result := ""
	for n > 0 {
		rm := n % int64(c.base)
		result = string(c.alphabet[rm]) + result
		n /= int64(c.base)
	}

	return result
}

// ConvertToInt -
func (c *Converter) ConvertToInt(s string) (int64, error) {
	var result int64 = 0
	length := len(s)
	for i := 0; i < length; i++ {
		idx := strings.IndexByte(c.alphabet, s[i])
		if idx == -1 {
			return 0, fmt.Errorf("%w: %q", "ErrInvalidCharacte", s[i])
		}
		result = result*c.base + int64(idx)
	}
	return result, nil
}
