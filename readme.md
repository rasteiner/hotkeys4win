# Hotkeys for Windows


I am a golang newbie and made this for learning purposes. 
You shouldn't be using this. You should be using a real library created by someone who actually knows what they're doing. 
A google search suggested this: https://github.com/golang-design/hotkey

If you too are learning, here's what I learned from doing this:

 - you register hotkeys with windows by calling the `RegisterHotKey` syscall from user32.dll, you tell it a modifiers bitmask (alt, shift, ctrl, etc), the virtual keycode for the hotkey, and a unique id (you invent that) for the specific hotkey
 - Windows will send you a message when the hotkey gets pressed (a `WM_HOTKEY` message. `WM_HOTKEY` stays for the number `0x0312`)
 - you listen for this message with `GetMessageW` (user32.dll), which is a blocking call (it waits until it receives a message)
 - **IMPORTANT:** Windows sends the message to the thread id which originally called `RegisterHotKey`. And only there.
   This means you need to make sure to register the hotkeys (call `RegisterHotKey`) and listen for messages (`GetMessageW`) in the same thread. You can make sure it stays the same thread by using a **single** goroutine that is locked to its OS thread (called `runtime.LockOSThread()`).
   
   The most ergonomic way I've found to do this is to create a "worker" thread that receives functions to execute via a channel. This way the functions are executed in the correct thread, but can still be written as closures in other parts of the program. (See `newWorker` function)
 - Since `GetMessageW` is blocking, and you need to both register, unregister and listen to messages in the same goroutine (and thread), I send dummy messages (`WM_USER`) to this thread after putting new functions to be executed on the queue. This allows `GetMessageW` to return without having a need for the user to actually press a hotkey and so I can check if there's more work to do other than waiting for messages.
 Other people solve this problem by using `PeekMessageW` instead of `GetMessageW`. PeekMessage isn't blocking so if there is no message it falls through and you continue normally with your tasks, however this means you need to continuosly poll for new messages (many times per second if you want a decent latency); I kinda found that wasteful since both "registering" and "hotkey presses" aren't normally really common events. 