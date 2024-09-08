package hotkeys

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	ModAlt = 1 << iota
	ModCtrl
	ModShift
	ModWin // Windows key

	WM_HOTKEY = 0x0312
	WM_USER   = 0x0400
)

type Hotkey struct {
	id         int32  // Unique identifier for the hotkey
	callback   func() // Function to call when hotkey is pressed
	keycode    int    // Keycode of the hotkey
	modifiers  int    // Modifiers of the hotkey
	registered bool   // Whether the hotkey is registered
}

type MSG struct {
	HWND   uintptr
	UINT   uintptr
	WPARAM int32
	LPARAM int64
	DWORD  int32
	POINT  struct{ X, Y int64 }
}

type syscallResult struct {
	r1  uintptr
	err error
}

var (
	inThread, threadId = newWorker()
	lastID             int32

	hotkeyMap map[int32]*Hotkey

	isListening bool

	user32            = syscall.NewLazyDLL("user32.dll")
	vkKeyScan         = user32.NewProc("VkKeyScanW")
	registerHotkey    = user32.NewProc("RegisterHotKey")
	unregisterHotkey  = user32.NewProc("UnregisterHotKey")
	getMessage        = user32.NewProc("GetMessageW")
	PeekMessage       = user32.NewProc("PeekMessageW")
	postThreadMessage = user32.NewProc("PostThreadMessageW")

	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	getCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
)

func newWorker() (chan func(), int) {
	cmds := make(chan func(), 20)

	threadChan := make(chan int)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// request a message queue for this thread by calling PeekMessage
		var msg MSG
		PeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)

		r1, _, _ := getCurrentThreadId.Call()
		threadChan <- int(r1)

		for cmd := range cmds {
			cmd()
		}
	}()

	return cmds, <-threadChan
}

// String returns a string representation of the hotkey
func (h *Hotkey) String() string {
	mod := &bytes.Buffer{}

	if h.modifiers&ModCtrl != 0 {
		mod.WriteString("Ctrl+")
	}
	if h.modifiers&ModAlt != 0 {
		mod.WriteString("Alt+")
	}
	if h.modifiers&ModShift != 0 {
		mod.WriteString("Shift+")
	}
	if h.modifiers&ModWin != 0 {
		mod.WriteString("Win+")
	}
	return fmt.Sprintf("Hotkey[Id: %d, %s%c]", h.id, mod, h.keycode)
}

// returns the modifier mask, the keycode and an error
func parse(keys string) (int, int, error) {

	keys = strings.ToLower(keys)
	// split the keys by '+'
	keyList := strings.Split(keys, "+")
	// initialize the modifier mask
	modifiers := 0
	// initialize the keycode
	keycode := 0

	// iterate over the keys
	for _, key := range keyList {
		// check if the key is a modifier
		switch key {
		case "alt":
			modifiers |= ModAlt
		case "ctrl":
			modifiers |= ModCtrl
		case "shift":
			modifiers |= ModShift
		case "win":
			modifiers |= ModWin
		default:
			if keycode != 0 {
				// we already have a keycode, return an error
				return 0, 0, fmt.Errorf("only one key allowed")
			}

			// check if the key is a letter
			if len(key) == 1 {
				// get the keycode for the key
				r1, _, _ := vkKeyScan.Call(uintptr(key[0]))
				keycode = int(r1)
			} else {
				// return an error if the key is not a letter
				return 0, 0, fmt.Errorf("invalid key: %s", key)
			}
		}
	}

	// return the modifier mask, the keycode and nil
	return modifiers, keycode, nil
}

// listenToHotkeys listens to hotkeys and sends the id of the hotkey that was pressed
func ListenToHotkeys() {
	if isListening {
		return
	}
	isListening = true

	go func() {

		for {
			done := make(chan MSG)
			inThread <- func() {
				var msg MSG
				getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
				done <- msg
			}
			msg := <-done

			if msg.UINT == WM_USER {
				// message from the thread to loop again so that I can check for register and unregister calls
				continue
			}

			if msg.UINT != WM_HOTKEY {
				// this makes the above check redundant, but I like the explicitness
				continue
			}

			hotkey := int32(msg.WPARAM)
			if hotkey == 0 {
				continue
			}

			if hotkeyMap[hotkey] != nil && hotkeyMap[hotkey].registered && hotkeyMap[hotkey].callback != nil {
				hk := hotkeyMap[hotkey]
				hk.callback()
			}
		}
	}()
}

func Register(keys string, callback func()) (*Hotkey, error) {
	mods, key, err := parse(keys)
	if err != nil {
		return nil, err
	}

	hk := &Hotkey{
		callback:  callback,
		keycode:   key,
		modifiers: mods,
		id:        lastID + 1,
	}

	done := make(chan syscallResult)

	inThread <- func() {
		r1, _, err := registerHotkey.Call(0, uintptr(hk.id), uintptr(hk.modifiers), uintptr(hk.keycode))
		done <- syscallResult{r1, err}
	}

	if isListening && threadId != 0 {
		// already listening, probably need to post a message to the thread so that it can loop again
		r1, _, err := postThreadMessage.Call(uintptr(threadId), WM_USER, 0, 0)
		if r1 == 0 {
			return nil, fmt.Errorf("error posting message to thread %v, because %v", threadId, err)
		}
	}

	result := <-done

	if result.r1 != 0 {
		lastID = hk.id
		hk.registered = true
	} else {
		return nil, fmt.Errorf("could not register hotkey (%s): %v", keys, err)
	}

	if hotkeyMap == nil {
		hotkeyMap = make(map[int32]*Hotkey)
	}

	hotkeyMap[hk.id] = hk

	return hk, nil
}

func Unregister(hk *Hotkey) error {
	if hk.registered {
		done := make(chan syscallResult)
		inThread <- func() {
			r1, _, err := unregisterHotkey.Call(uintptr(0), uintptr(hk.id))
			done <- syscallResult{r1, err}
		}

		if isListening && threadId != 0 {
			// already listening, probably need to post a message to the thread so that it can loop again
			postThreadMessage.Call(uintptr(threadId), WM_USER, 0, 0)
		}

		result := <-done

		if result.r1 != 0 {
			fmt.Println("Hotkey unregistered")
			hk.registered = false
			delete(hotkeyMap, hk.id)
		} else {
			return fmt.Errorf("could not unregister hotkey: %v", result.err)
		}
	}
	return nil
}
