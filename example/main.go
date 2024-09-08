package main

import (
	"context"
	"fmt"
	"time"

	hotkeys "github.com/rasteiner/hotkeys4win"
)

func main() {
	ctx := context.Background()

	timeout, cancel := context.WithTimeout(ctx, 7*time.Second)

	// call this function sometime after or before registering hotkeys to start listening for them
	hotkeys.ListenToHotkeys()

	// register hotkeys with the Register function,
	// give it a string with the keys you want to register and a callback function
	hk, err := hotkeys.Register("ctrl+shift+z", func() {
		fmt.Println("control shift z")
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(hk)

	// you can register multiple hotkeys
	hk, err = hotkeys.Register("ctrl+shift+y", func() {
		fmt.Println("control shift y")
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(hk)

	// you can all the modifiers you want, but the key must be a single character
	hk, err = hotkeys.Register("ctrl+shift+alt+win+r", func() {
		fmt.Println("wow such button")
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(hk)

	<-timeout.Done()
	cancel()
	fmt.Println("Timeout reached")

}
