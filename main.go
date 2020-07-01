package main

import (
	"context"
	"io/ioutil"
	"math"
	"os"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"
)

var (
	moduser32       = syscall.NewLazyDLL("user32.dll")
	procGetKeyState = moduser32.NewProc("GetKeyState")
	mainApp         = Main{
		timeout: 1000 * time.Millisecond,
		active:  true,
	}
)

// Theres some race condition somewhere causing wierd Qt behaviour which this fixes for some reason
func log(t string) {
	time.Sleep(100 * time.Millisecond)
}

// Main represents the Qt application and the label
type Main struct {
	app        *widgets.QApplication
	window     *widgets.QMainWindow
	label      *widgets.QLabel
	lastUpdate time.Time
	timeout    time.Duration

	loop    chan bool
	running context.Context
	stop    func()

	active bool
}

// UpdateImage a new image in overlay
func (app *Main) UpdateImage(path string) {
	app.lastUpdate = time.Now().UTC()

	app.window.SetWindowOpacity(100 / 100)

	app.stop()
	select {
	case <-app.running.Done():
		app.running, app.stop = context.WithCancel(context.Background())
	}

	// We will do this in the event the resolution of the primary screen changes
	size := gui.QGuiApplication_PrimaryScreen().Size()
	width := size.Width()
	height := size.Height()
	bound := int(math.Floor(float64(width) / 10))

	_pixmap := gui.NewQPixmap3(path, "", core.Qt__NoFormatConversion)
	pixmap := _pixmap.Scaled2(bound, bound, core.Qt__KeepAspectRatio, core.Qt__SmoothTransformation)
	app.label.SetPixmap(pixmap)

	// Resize the window to fit the new image
	app.window.Resize(pixmap.Size())
	app.window.Show()

	padding := math.Floor(float64(height) / 24)
	app.window.Move2(
		int(width-pixmap.Width()-int(padding)),
		int(padding),
	)
	time.Sleep(app.timeout)

	if time.Now().UTC().Sub(app.lastUpdate) >= app.timeout {
		// We called another update image whilst waiting
		if ok := app.Fadeout(); ok {
			app.window.Hide()
		}
	}
}

// GetKeyState beep boop keyboard things
func GetKeyState(vKey int) uint16 {
	ret, _, _ := procGetKeyState.Call(uintptr(vKey))
	return uint16(ret)
}

// ReadKeys handles detecting numlock, scrolllock, capslock
func (app *Main) ReadKeys() {
	t := time.NewTicker(5 * time.Millisecond)

	// If the last bit is 1, it is active
	capsheld := GetKeyState(0x14)&(2<<15-1) == 1
	numheld := GetKeyState(0x90)&(2<<15-1) == 1
	scrollheld := GetKeyState(0x91)&(2<<15-1) == 1

	for {
		_capsheld := GetKeyState(0x14)&(2<<15-1) == 1
		if _capsheld != capsheld {
			if _capsheld {
				go app.UpdateImage("capslock_on.svg")
			} else {
				go app.UpdateImage("capslock_off.svg")
			}
			capsheld = _capsheld
		}

		_numheld := GetKeyState(0x90)&(2<<15-1) == 1
		if _numheld != numheld {
			if _numheld {
				go app.UpdateImage("numlock_on.svg")
			} else {
				go app.UpdateImage("numlock_off.svg")
			}
			numheld = _numheld
		}

		_scrollheld := GetKeyState(0x91)&(2<<15-1) == 1
		if _scrollheld != scrollheld {
			if _scrollheld {
				go app.UpdateImage("scrolllock_on.svg")
			} else {
				go app.UpdateImage("scrolllock_off.svg")
			}
			scrollheld = _scrollheld
		}

		select {
		case <-app.loop:
			return
		case <-t.C:
			continue
		}
	}
}

// Fadeout fades the overlay instead of immediately disappearing
func (app *Main) Fadeout() bool {
	t := time.NewTicker(40 * time.Millisecond)
	for i := 25; i > 0; i-- {
		app.window.SetWindowOpacity(float64(i*4) / 100)
		select {
		case <-app.running.Done():
			return false
		case <-t.C:
			continue
		}
	}
	return true
}

func onSystrayReady() {
	log("Systray Ready")

	systray.SetTitle("KeyDisplay")
	systray.SetTooltip("Show when you are using keyboard shortcuts")

	icon, _ := ioutil.ReadFile("icon.ico")
	systray.SetIcon(icon)

	mQuit := systray.AddMenuItem("Quit", "Close application")
	mToggle := systray.AddMenuItem("Toggle", "Hide overlay")

	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				go mainApp.UpdateImage("keydisplay_off.svg")
				close(mainApp.loop)
				time.Sleep(mainApp.timeout)
				os.Exit(0)
			case <-mToggle.ClickedCh:
				if mainApp.active {
					go mainApp.UpdateImage("keydisplay_off.svg")
					close(mainApp.loop)
				} else {
					go mainApp.UpdateImage("keydisplay_on.svg")
					mainApp.loop = make(chan bool, 1)
					go mainApp.ReadKeys()
				}
				mainApp.active = !mainApp.active
			}
		}
	}()
}

func onSystrayExit() {
	systray.Quit()
}

func main() {
	log("Make App")
	mainApp.app = widgets.NewQApplication(len(os.Args), os.Args)
	mainApp.loop = make(chan bool, 1)

	log("Make CTX")
	mainApp.running, mainApp.stop = context.WithCancel(context.Background())

	log("Make Window")
	mainApp.window = widgets.NewQMainWindow(nil, core.Qt__FramelessWindowHint|core.Qt__WindowStaysOnTopHint)
	mainApp.window.SetAttribute(core.Qt__WA_TranslucentBackground, true)
	mainApp.window.SetWindowTitle("KeyDisplay")

	log("Make Label")
	mainApp.label = widgets.NewQLabel(mainApp.window, core.Qt__Widget)
	mainApp.window.SetCentralWidget(mainApp.label)

	log("Goroutine")
	go func() {
		systray.Run(onSystrayReady, onSystrayExit)
	}()
	go mainApp.UpdateImage("keydisplay_on.svg")
	go mainApp.ReadKeys()

	log("QAP Exec")
	widgets.QApplication_Exec()
}
