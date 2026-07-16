package main

import (
	"fmt"
	"urlshort/pkg/utils"
)

// start
func main() {

	const ss string = "5aA1bB2CcdD3Ee4Ff0gG6Hh7iI8jJ9kKzLsMtNuOvPwQxSyTrUqVpWoXnYmZl"
	//const ss string = "0123456789ABCDEFGHIJKLMNOPQSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	cm := utils.NewConverter(ss)

	for i := int64(1000000); i > 0; i-- {

		v := cm.ConvertToStr(i)
		v1 := cm.ConvertToInt(v)
		fmt.Println(i, v1, v)
		if v1 != i {
			fmt.Println(i, v1, v)
			panic(i)
		}
	}
}
