package sync

import (
	"fmt"

	"github.com/gookit/color"
)

func sendDispChanged(path string) {
	fmt.Print("⫸ ")
	color.New(color.FgLightGreen, color.Bold).Print(" Changed ")
	color.Gray.Println(path)
}

func sendDispDeleted(path string) {
	fmt.Print("⫸ ")
	color.New(color.FgLightRed, color.Bold).Print(" Deleted ")
	color.Gray.Println(path)
}

func recvDispChanged(path string) {
	fmt.Print("⫷ ")
	color.New(color.FgLightGreen, color.Bold).Print(" Changed ")
	color.Gray.Println(path)
}

func recvDispDeleted(path string) {
	fmt.Print("⫷ ")
	color.New(color.FgLightRed, color.Bold).Print(" Deleted ")
	color.Gray.Println(path)
}
