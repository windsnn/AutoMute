package main

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed app.ico
var embeddedIconBytes []byte

// Win32 API DLLs
var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")
	modOle32    = syscall.NewLazyDLL("ole32.dll")
	modComdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")

	// User32
	procDefWindowProc            = modUser32.NewProc("DefWindowProcW")
	procRegisterClassEx          = modUser32.NewProc("RegisterClassExW")
	procCreateWindowEx           = modUser32.NewProc("CreateWindowExW")
	procShowWindow               = modUser32.NewProc("ShowWindow")
	procUpdateWindow             = modUser32.NewProc("UpdateWindow")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procGetForegroundWindow      = modUser32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcId    = modUser32.NewProc("GetWindowThreadProcessId")
	procPostQuitMessage          = modUser32.NewProc("PostQuitMessage")
	procGetMessage               = modUser32.NewProc("GetMessageW")
	procTranslateMessage         = modUser32.NewProc("TranslateMessage")
	procDispatchMessage          = modUser32.NewProc("DispatchMessageW")
	procPostMessage              = modUser32.NewProc("PostMessageW")
	procSendMessage              = modUser32.NewProc("SendMessageW")
	procSetWinEventHook          = modUser32.NewProc("SetWinEventHook")
	procUnhookWinEvent           = modUser32.NewProc("UnhookWinEvent")
	procGetCursorPos             = modUser32.NewProc("GetCursorPos")
	procCreatePopupMenu          = modUser32.NewProc("CreatePopupMenu")
	procAppendMenu               = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu           = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu              = modUser32.NewProc("DestroyMenu")
	procLoadImage                = modUser32.NewProc("LoadImageW")
	procLoadIcon                 = modUser32.NewProc("LoadIconW")
	procCreateIconFromResourceEx = modUser32.NewProc("CreateIconFromResourceEx")
	procMessageBox               = modUser32.NewProc("MessageBoxW")
	procSetWindowText            = modUser32.NewProc("SetWindowTextW")
	procGetWindowText            = modUser32.NewProc("GetWindowTextW")
	procSetTimer                 = modUser32.NewProc("SetTimer")
	procKillTimer                = modUser32.NewProc("KillTimer")
	procDestroyWindow            = modUser32.NewProc("DestroyWindow")
	procLoadCursor               = modUser32.NewProc("LoadCursorW")
	procFindWindow               = modUser32.NewProc("FindWindowW")

	// Gdi32
	procCreateFont     = modGdi32.NewProc("CreateFontW")
	procGetStockObject = modGdi32.NewProc("GetStockObject")

	// Shell32
	procShellNotifyIcon = modShell32.NewProc("Shell_NotifyIconW")
	procShellExecute    = modShell32.NewProc("ShellExecuteW")

	// Kernel32
	procGetModuleHandle           = modKernel32.NewProc("GetModuleHandleW")
	procOpenProcess               = modKernel32.NewProc("OpenProcess")
	procCloseHandle               = modKernel32.NewProc("CloseHandle")
	procQueryFullProcessImageName = modKernel32.NewProc("QueryFullProcessImageNameW")
	procCreateToolhelp32Snapshot  = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First            = modKernel32.NewProc("Process32FirstW")
	procProcess32Next             = modKernel32.NewProc("Process32NextW")
	procCreateMutex               = modKernel32.NewProc("CreateMutexW")
	procGetLastError              = modKernel32.NewProc("GetLastError")

	// Comdlg32
	procGetOpenFileName = modComdlg32.NewProc("GetOpenFileNameW")

	// Advapi32 (Registry Autostart)
	procRegOpenKeyEx    = modAdvapi32.NewProc("RegOpenKeyExW")
	procRegSetValueEx   = modAdvapi32.NewProc("RegSetValueExW")
	procRegDeleteValue  = modAdvapi32.NewProc("RegDeleteValueW")
	procRegQueryValueEx = modAdvapi32.NewProc("RegQueryValueExW")
	procRegCloseKey     = modAdvapi32.NewProc("RegCloseKey")

	// Ole32
	procCoInitialize   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInst   = modOle32.NewProc("CoCreateInstance")
	procCoUninitialize = modOle32.NewProc("CoUninitialize")
)

// Win32 Constants
const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_THICKFRAME       = 0x00040000
	WS_MAXIMIZEBOX      = 0x00010000
	ES_AUTOHSCROLL      = 0x00000080

	BS_AUTOCHECKBOX = 0x00000003
	BS_PUSHBUTTON   = 0x00000000
	LBS_NOTIFY      = 0x00000001
	LB_ADDSTRING    = 0x0180
	LB_RESETCONTENT = 0x0184
	LB_GETCURSEL    = 0x0188
	LB_GETTEXT      = 0x0189
	BM_SETCHECK     = 0x00F1
	BST_CHECKED     = 0x0001
	BST_UNCHECKED   = 0x0000

	WM_DESTROY         = 0x0002
	WM_ACTIVATE        = 0x0006
	WM_CLOSE           = 0x0010
	WM_QUERYENDSESSION = 0x0011
	WM_ENDSESSION      = 0x0016
	WM_COMMAND         = 0x0111
	WM_TIMER           = 0x0113
	WM_SETFONT         = 0x0030
	WM_APP             = 0x8000
	WM_TRAYICON        = WM_APP + 1
	WM_LBUTTONUP       = 0x0202
	WM_LBUTTONDBL      = 0x0203
	WM_RBUTTONUP       = 0x0205

	IDT_FOCUS_TIMER = 1

	SW_HIDE    = 0
	SW_SHOW    = 5
	SW_RESTORE = 9

	NIM_ADD    = 0x00000000
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008

	TPM_BOTTOMALIGN = 0x0020
	TPM_RIGHTALIGN  = 0x0008

	EVENT_SYSTEM_FOREGROUND = 0x0003
	WINEVENT_OUTOFCONTEXT   = 0x0000
	WINEVENT_SKIPOWNPROCESS = 0x0002

	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x0010
	LR_DEFAULTSIZE  = 0x0040

	IDI_APPLICATION = 32512
	IDC_ARROW       = 32512

	HKEY_CURRENT_USER = 0x80000001
	KEY_READ          = 0x20019
	KEY_WRITE         = 0x20006
	REG_SZ            = 1

	ERROR_ALREADY_EXISTS = 183

	OFN_EXPLORER      = 0x00080000
	OFN_FILEMUSTEXIST = 0x00001000
	OFN_PATHMUSTEXIST = 0x00000800
	OFN_NOCHANGEDIR   = 0x00000008
)

// UI Control IDs
const (
	IDC_LST_MANAGED = 101 + iota
	IDC_EDT_PROCESS
	IDC_BTN_BROWSE
	IDC_BTN_ADD
	IDC_BTN_ADDFG
	IDC_BTN_REMOVE
	IDC_BTN_CLEAR
	IDC_BTN_OPEN_CONFIG
	IDC_CHK_ENABLE
	IDC_CHK_FADE
	IDC_CHK_AUTOSTART
	IDC_LBL_STATUS
)

// Tray Menu IDs
const (
	IDM_TRAY_SHOW = 201 + iota
	IDM_TRAY_TOGGLE
	IDM_TRAY_FADE
	IDM_TRAY_AUTOSTART
	IDM_TRAY_EXIT
)

type GUID [16]byte

var (
	CLSID_MMDeviceEnumerator  = GUID{149, 3, 222, 188, 47, 229, 124, 70, 142, 61, 196, 87, 146, 145, 105, 46}
	IID_IMMDeviceEnumerator   = GUID{210, 100, 86, 169, 20, 150, 53, 79, 167, 70, 222, 141, 182, 54, 23, 230}
	IID_IAudioSessionManager2 = GUID{160, 153, 170, 119, 214, 27, 79, 72, 139, 199, 44, 101, 76, 154, 155, 111}
	IID_ISimpleAudioVolume    = GUID{152, 84, 206, 135, 214, 104, 229, 68, 146, 21, 109, 164, 126, 248, 131, 216}
	IID_IAudioSessionControl2 = GUID{136, 255, 183, 191, 57, 114, 201, 79, 143, 162, 7, 201, 80, 190, 156, 109}
)

type POINT struct {
	X, Y int32
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	TimeoutOrVersion uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
	HBalloonIcon     uintptr
}

type OPENFILENAMEW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

type ConfigData struct {
	Enabled   bool     `json:"enabled"`
	Fade      bool     `json:"fade"`
	Processes []string `json:"processes"`
}

type App struct {
	config            ConfigData
	configPath        string
	lastConfigModTime time.Time
	managed           map[string]bool
	currentFg         string
	externalFg        string // Tracks the most recent non-AutoMute foreground application
	selfName          string // Lowercased name of AutoMute executable

	hwnd   uintptr
	nid    NOTIFYICONDATAW
	hHook  uintptr
	hMutex uintptr
	exited bool

	// UI Controls (only dynamic controls requiring runtime access)
	lstManaged uintptr
	edtProcess uintptr
	btnAddFg   uintptr
	chkEnable  uintptr
	chkFade    uintptr
	chkAuto    uintptr
	lblStatus  uintptr

	// Fade Concurrency State
	fadeMu  sync.Mutex
	fadeSeq map[string]uint64
}

var app = App{
	fadeSeq: make(map[string]uint64),
}

func callCOM(obj uintptr, index int, args ...uintptr) (uintptr, uintptr, error) {
	if obj == 0 {
		return 0, 0, syscall.EINVAL
	}
	pObj := *(***[100]uintptr)(unsafe.Pointer(&obj))
	if pObj == nil {
		return 0, 0, syscall.EINVAL
	}
	pVtbl := *pObj
	if pVtbl == nil {
		return 0, 0, syscall.EINVAL
	}
	method := pVtbl[index]
	allArgs := make([]uintptr, 1+len(args))
	allArgs[0] = obj
	copy(allArgs[1:], args)
	return syscall.SyscallN(method, allArgs...)
}

type PROCESSENTRY32W struct {
	DwSize              uint32
	CntUsage            uint32
	Th32ProcessID       uint32
	Th32DefaultHeapID   uintptr
	Th32ModuleID        uint32
	CntThreads          uint32
	Th32ParentProcessID uint32
	PcPriClassBase      int32
	DwFlags             uint32
	SzExeFile           [260]uint16
}

func getProcNameByToolhelp(pid uint32) string {
	const TH32CS_SNAPPROCESS = 0x00000002
	h, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if h == 0 || h == ^uintptr(0) {
		return ""
	}
	defer procCloseHandle.Call(h)
	var pe PROCESSENTRY32W
	pe.DwSize = uint32(unsafe.Sizeof(pe))
	ret, _, _ := procProcess32First.Call(h, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		if pe.Th32ProcessID == pid {
			return syscall.UTF16ToString(pe.SzExeFile[:])
		}
		ret, _, _ = procProcess32Next.Call(h, uintptr(unsafe.Pointer(&pe)))
	}
	return ""
}

func getProcessName(pid uint32) string {
	if pid == 0 || pid == 4 {
		return "system"
	}
	if pid == uint32(os.Getpid()) {
		return "automute.exe"
	}
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if h != 0 {
		var buf [1024]uint16
		size := uint32(len(buf))
		ret, _, _ := procQueryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		procCloseHandle.Call(h)
		if ret != 0 {
			path := syscall.UTF16ToString(buf[:size])
			return strings.ToLower(filepath.Base(path))
		}
	}
	// Fallback to Toolhelp32 snapshot for elevated/protected processes
	if name := getProcNameByToolhelp(pid); name != "" {
		return strings.ToLower(filepath.Base(name))
	}
	return fmt.Sprintf("[elevated:%d]", pid)
}

func getForegroundProcess() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "system"
	}
	var pid uint32
	procGetWindowThreadProcId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == uint32(os.Getpid()) {
		return "automute.exe"
	}
	return getProcessName(pid)
}

// Registry Autostart Operations
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func isAutostartEnabled() bool {
	var hKey uintptr
	keyNamePtr := syscall.StringToUTF16Ptr(runKeyPath)
	ret, _, _ := procRegOpenKeyEx.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(keyNamePtr)), 0, KEY_READ, uintptr(unsafe.Pointer(&hKey)))
	if ret != 0 {
		return false
	}
	defer procRegCloseKey.Call(hKey)

	valNamePtr := syscall.StringToUTF16Ptr("AutoMute")
	var valType uint32
	ret, _, _ = procRegQueryValueEx.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)), 0, uintptr(unsafe.Pointer(&valType)), 0, 0)
	return ret == 0
}

func setAutostart(enable bool) {
	var hKey uintptr
	keyNamePtr := syscall.StringToUTF16Ptr(runKeyPath)
	ret, _, _ := procRegOpenKeyEx.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(keyNamePtr)), 0, KEY_WRITE, uintptr(unsafe.Pointer(&hKey)))
	if ret != 0 {
		return
	}
	defer procRegCloseKey.Call(hKey)

	valNamePtr := syscall.StringToUTF16Ptr("AutoMute")
	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return
		}
		valBytes := syscall.StringToUTF16(fmt.Sprintf("\"%s\" --minimized", exePath))
		procRegSetValueEx.Call(
			hKey,
			uintptr(unsafe.Pointer(valNamePtr)),
			0,
			REG_SZ,
			uintptr(unsafe.Pointer(&valBytes[0])),
			uintptr(len(valBytes)*2),
		)
	} else {
		procRegDeleteValue.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)))
	}
}

func shouldStartMinimized(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(strings.TrimSpace(arg))
		if lower == "--minimized" || lower == "-minimized" || lower == "--silent" || lower == "-silent" || lower == "--tray" || lower == "-tray" || lower == "/minimized" || lower == "/silent" {
			return true
		}
	}
	return false
}

func (a *App) loadConfig() {
	a.managed = make(map[string]bool)
	if fi, err := os.Stat(a.configPath); err == nil {
		a.lastConfigModTime = fi.ModTime()
	}
	data, err := os.ReadFile(a.configPath)
	if err == nil {
		var cfg ConfigData
		if err := json.Unmarshal(data, &cfg); err == nil {
			a.config = cfg
			cleanProcs := make([]string, 0, len(cfg.Processes))
			for _, p := range cfg.Processes {
				cp := cleanProcessName(p)
				if cp != "" && !isIgnoredExternalApp(cp) {
					a.managed[cp] = true
					cleanProcs = append(cleanProcs, cp)
				}
			}
			sort.Strings(cleanProcs)
			a.config.Processes = cleanProcs
			return
		}
	}

	a.config = ConfigData{
		Enabled:   true,
		Fade:      true,
		Processes: []string{"msedge.exe"},
	}
	a.managed["msedge.exe"] = true
	if os.IsNotExist(err) {
		a.saveConfig()
	}
}

func (a *App) saveConfig() {
	procs := make([]string, 0, len(a.managed))
	for p := range a.managed {
		procs = append(procs, p)
	}
	sort.Strings(procs)
	a.config.Processes = procs

	if data, err := json.MarshalIndent(a.config, "", "    "); err == nil {
		if dir := filepath.Dir(a.configPath); dir != "" {
			_ = os.MkdirAll(dir, 0755)
		}
		if err := os.WriteFile(a.configPath, data, 0644); err == nil {
			if fi, err := os.Stat(a.configPath); err == nil {
				a.lastConfigModTime = fi.ModTime()
			}
		}
	}
}

func (a *App) checkReloadConfig() {
	if a.configPath == "" {
		return
	}
	fi, err := os.Stat(a.configPath)
	if err != nil {
		return
	}
	if !fi.ModTime().Equal(a.lastConfigModTime) {
		a.loadConfig()
		a.applyMuteState(a.config.Fade)
		a.refreshListBox()
	}
}

// iterateProcessAudioVolumes enumerates WASAPI sessions and invokes fn with ISimpleAudioVolume for matching processes.
func iterateProcessAudioVolumes(targetProc string, fn func(pVol uintptr)) {
	var pEnum uintptr
	hr, _, _ := procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&CLSID_MMDeviceEnumerator)),
		0,
		1,
		uintptr(unsafe.Pointer(&IID_IMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&pEnum)),
	)
	if int32(hr) < 0 || pEnum == 0 {
		return
	}
	defer callCOM(pEnum, 2)

	var pDev uintptr
	hr, _, _ = callCOM(pEnum, 4, 0, 1, uintptr(unsafe.Pointer(&pDev)))
	if int32(hr) < 0 || pDev == 0 {
		// Fallback to eConsole (0) if eMultimedia (1) is unavailable
		hr, _, _ = callCOM(pEnum, 4, 0, 0, uintptr(unsafe.Pointer(&pDev)))
		if int32(hr) < 0 || pDev == 0 {
			return
		}
	}
	defer callCOM(pDev, 2)

	var pMgr uintptr
	hr, _, _ = callCOM(pDev, 3, uintptr(unsafe.Pointer(&IID_IAudioSessionManager2)), 1, 0, uintptr(unsafe.Pointer(&pMgr)))
	if int32(hr) < 0 || pMgr == 0 {
		return
	}
	defer callCOM(pMgr, 2)

	var pSessEnum uintptr
	hr, _, _ = callCOM(pMgr, 5, uintptr(unsafe.Pointer(&pSessEnum)))
	if int32(hr) < 0 || pSessEnum == 0 {
		return
	}
	defer callCOM(pSessEnum, 2)

	var count int32
	callCOM(pSessEnum, 3, uintptr(unsafe.Pointer(&count)))

	for i := int32(0); i < count; i++ {
		var pSessCtrl uintptr
		callCOM(pSessEnum, 4, uintptr(i), uintptr(unsafe.Pointer(&pSessCtrl)))
		if pSessCtrl == 0 {
			continue
		}

		var pCtrl2 uintptr
		hr, _, _ = callCOM(pSessCtrl, 0, uintptr(unsafe.Pointer(&IID_IAudioSessionControl2)), uintptr(unsafe.Pointer(&pCtrl2)))
		if int32(hr) < 0 || pCtrl2 == 0 {
			callCOM(pSessCtrl, 2)
			continue
		}
		var pid uint32
		callCOM(pCtrl2, 14, uintptr(unsafe.Pointer(&pid)))
		callCOM(pCtrl2, 2)

		pName := getProcessName(pid)
		if strings.EqualFold(pName, targetProc) {
			var pVol uintptr
			hr, _, _ = callCOM(pSessCtrl, 0, uintptr(unsafe.Pointer(&IID_ISimpleAudioVolume)), uintptr(unsafe.Pointer(&pVol)))
			if int32(hr) >= 0 && pVol != 0 {
				fn(pVol)
				callCOM(pVol, 2)
			}
		}
		callCOM(pSessCtrl, 2)
	}
}

func (a *App) getProcessVolume(targetProc string) float64 {
	var vol float64
	found := false
	iterateProcessAudioVolumes(targetProc, func(pVol uintptr) {
		if !found {
			var curVol float32
			callCOM(pVol, 4, uintptr(unsafe.Pointer(&curVol)))
			vol = float64(curVol)
			found = true
		}
	})
	return vol
}

func (a *App) setProcessVolume(targetProc string, targetVol float64) {
	bits := uintptr(math.Float32bits(float32(targetVol)))
	iterateProcessAudioVolumes(targetProc, func(pVol uintptr) {
		callCOM(pVol, 3, bits, 0)
	})
}

// smoothStep computes the smoothstep S-curve transition between 0.0 and 1.0.
func smoothStep(t float64) float64 {
	if t <= 0.0 {
		return 0.0
	}
	if t >= 1.0 {
		return 1.0
	}
	return t * t * (3.0 - 2.0*t)
}

func (a *App) cancelFade(proc string) {
	a.fadeMu.Lock()
	if a.fadeSeq != nil {
		a.fadeSeq[proc]++
		lower := strings.ToLower(proc)
		if lower != proc {
			a.fadeSeq[lower] = a.fadeSeq[proc]
		}
	}
	a.fadeMu.Unlock()
}

func (a *App) cancelAllFades() {
	a.fadeMu.Lock()
	for k := range a.fadeSeq {
		a.fadeSeq[k]++
	}
	a.fadeMu.Unlock()
}

// Smooth Independent Fade In/Out via High-Precision Goroutine with sequence breaker.
func (a *App) fadeProcess(proc string, targetVol float64, duration time.Duration) {
	if duration <= 0 {
		a.cancelFade(proc)
		a.setProcessVolume(proc, targetVol)
		return
	}
	a.fadeMu.Lock()
	if a.fadeSeq == nil {
		a.fadeSeq = make(map[string]uint64)
	}
	a.fadeSeq[proc]++
	curSeq := a.fadeSeq[proc]
	lower := strings.ToLower(proc)
	if lower != proc {
		a.fadeSeq[lower] = curSeq
	}
	a.fadeMu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Initialize COM in MTA for worker audio thread
		procCoInitialize.Call(0, 0)
		defer procCoUninitialize.Call()

		startVol := a.getProcessVolume(proc)
		if math.Abs(startVol-targetVol) < 0.001 {
			a.setProcessVolume(proc, targetVol)
			return
		}

		// 25 steps over duration (e.g. 1000ms -> 40ms per step, smooth 25 FPS)
		steps := 25
		stepDur := duration / time.Duration(steps)

		for i := 1; i <= steps; i++ {
			time.Sleep(stepDur)

			a.fadeMu.Lock()
			if a.fadeSeq[proc] != curSeq {
				a.fadeMu.Unlock()
				return // Superseded by a newer focus switch or cancellation
			}
			a.fadeMu.Unlock()

			t := float64(i) / float64(steps)
			vol := startVol + (targetVol-startVol)*smoothStep(t)
			if vol < 0.0 {
				vol = 0.0
			} else if vol > 1.0 {
				vol = 1.0
			}
			a.setProcessVolume(proc, vol)
		}

		a.fadeMu.Lock()
		if a.fadeSeq[proc] != curSeq {
			a.fadeMu.Unlock()
			return
		}
		a.fadeMu.Unlock()
		a.setProcessVolume(proc, targetVol)
	}()
}

func (a *App) restoreAllManaged() {
	a.cancelAllFades()
	for p := range a.managed {
		a.setProcessVolume(p, 1.0)
	}
}

func (a *App) applyMuteState(useFade bool) {
	if !a.config.Enabled {
		a.restoreAllManaged()
		return
	}

	fadeDuration := 1000 * time.Millisecond
	for p := range a.managed {
		isFg := strings.EqualFold(p, a.currentFg)
		targetVol := 0.0
		if isFg {
			targetVol = 1.0
		}

		if useFade && a.config.Fade {
			a.fadeProcess(p, targetVol, fadeDuration)
		} else {
			a.cancelFade(p)
			a.setProcessVolume(p, targetVol)
		}
	}
}

func isIgnoredExternalApp(name string) bool {
	if name == "" || strings.HasPrefix(name, "[") {
		return true
	}
	lower := strings.ToLower(name)
	if lower == ".exe" {
		return true
	}
	baseSelf := strings.TrimSuffix(app.selfName, ".exe")
	if lower == app.selfName || (baseSelf != "" && (lower == baseSelf || lower == baseSelf+".exe")) {
		return true
	}
	switch lower {
	case "automute.exe", "automute", "system", "system.exe", "explorer.exe", "explorer", "searchhost.exe", "shellexperiencehost.exe", "startmenuexperiencehost.exe":
		return true
	default:
		return false
	}
}

func (a *App) onForegroundChanged(fgProc string) {
	if !a.config.Enabled {
		return
	}

	fgProc = strings.ToLower(strings.TrimSpace(fgProc))
	if fgProc == "" {
		return
	}

	if !isIgnoredExternalApp(fgProc) {
		a.externalFg = fgProc
		a.updateAddFgButton()
		if a.edtProcess != 0 {
			procSetWindowText.Call(a.edtProcess, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(fgProc))))
		}
	}

	if fgProc == a.currentFg {
		return
	}
	prev := a.currentFg
	a.currentFg = fgProc

	fadeDuration := 1000 * time.Millisecond

	// 1. Mute previous app if managed
	if prev != "" && a.managed[prev] {
		if a.config.Fade {
			a.fadeProcess(prev, 0.0, fadeDuration)
		} else {
			a.cancelFade(prev)
			a.setProcessVolume(prev, 0.0)
		}
	}

	// 2. Unmute current app if managed
	if a.managed[fgProc] {
		if a.config.Fade {
			a.fadeProcess(fgProc, 1.0, fadeDuration)
		} else {
			a.cancelFade(fgProc)
			a.setProcessVolume(fgProc, 1.0)
		}
	}

	a.updateStatusText()
}

func (a *App) updateStatusText() {
	if a.lblStatus == 0 {
		return
	}
	status := "运行中"
	if !a.config.Enabled {
		status = "已暂停静音"
	}
	target := a.externalFg
	if target == "" {
		target = "无 (切到目标软件后再切回即可识别)"
	} else if a.managed[target] {
		target = target + " [已在静音列表中]"
	} else {
		target = target + " [未接管，可点击一键添加]"
	}
	txt := fmt.Sprintf("状态: %s  |  捕获的目标软件: %s  |  已接管: %d 个", status, target, len(a.managed))
	procSetWindowText.Call(a.lblStatus, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(txt))))
}

func (a *App) updateAddFgButton() {
	if a.btnAddFg == 0 {
		return
	}
	label := "+ 一键添加当前前台应用"
	if a.externalFg != "" {
		if a.managed[a.externalFg] {
			label = fmt.Sprintf("✓ 已接管: %s (点击取消接管)", a.externalFg)
		} else {
			label = fmt.Sprintf("+ 一键添加并静音: %s [未接管]", a.externalFg)
		}
	}
	procSetWindowText.Call(a.btnAddFg, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(label))))
}

func boolToCheck(b bool) uintptr {
	if b {
		return BST_CHECKED
	}
	return BST_UNCHECKED
}

func (a *App) refreshListBox() {
	procSendMessage.Call(a.lstManaged, LB_RESETCONTENT, 0, 0)
	procs := make([]string, 0, len(a.managed))
	for p := range a.managed {
		procs = append(procs, p)
	}
	sort.Strings(procs)
	for _, p := range procs {
		procSendMessage.Call(a.lstManaged, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(p))))
	}
	procSendMessage.Call(a.chkEnable, BM_SETCHECK, boolToCheck(a.config.Enabled), 0)
	procSendMessage.Call(a.chkFade, BM_SETCHECK, boolToCheck(a.config.Fade), 0)
	procSendMessage.Call(a.chkAuto, BM_SETCHECK, boolToCheck(isAutostartEnabled()), 0)
	a.updateStatusText()
	a.updateAddFgButton()
}

func winEventProc(hWinEventHook uintptr, event uint32, hwnd uintptr, idObject int32, idChild int32, idEventThread uint32, dwmsEventTime uint32) uintptr {
	if event == EVENT_SYSTEM_FOREGROUND {
		var pid uint32
		procGetWindowThreadProcId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		name := getProcessName(pid)
		app.onForegroundChanged(name)
	}
	return 0
}

func cleanProcessName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	name = filepath.Base(name)
	name = strings.Trim(name, `/\ `)
	if name == "" || strings.Trim(name, ".") == "" || name == ".exe" || strings.Contains(name, ":") {
		return ""
	}
	if !strings.HasSuffix(name, ".exe") {
		name += ".exe"
	}
	if name == ".exe" {
		return ""
	}
	return name
}

func browseExe(hwnd uintptr) string {
	var fileBuf [1024]uint16
	filter := []uint16{
		'可', '执', '行', '程', '序', ' ', '(', '*', '.', 'e', 'x', 'e', ')', 0,
		'*', '.', 'e', 'x', 'e', 0,
		'所', '有', '文', '件', ' ', '(', '*', '.', '*', ')', 0,
		'*', '.', '*', 0,
		0,
	}

	ofn := OPENFILENAMEW{
		HwndOwner:   hwnd,
		LpstrFilter: &filter[0],
		LpstrFile:   &fileBuf[0],
		NMaxFile:    uint32(len(fileBuf)),
		LpstrTitle:  syscall.StringToUTF16Ptr("选择要静音的应用程序 (.exe)"),
		Flags:       OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST | OFN_NOCHANGEDIR,
	}
	ofn.LStructSize = uint32(unsafe.Sizeof(ofn))

	ret, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret != 0 {
		fullPath := syscall.UTF16ToString(fileBuf[:])
		return strings.ToLower(filepath.Base(fullPath))
	}
	return ""
}

func (a *App) exit() {
	if a.exited {
		return
	}
	a.exited = true

	a.cancelAllFades()
	a.restoreAllManaged()
	h := a.hwnd
	a.hwnd = 0
	if h != 0 {
		procKillTimer.Call(h, IDT_FOCUS_TIMER)
		procDestroyWindow.Call(h)
	}
	if a.hHook != 0 {
		procUnhookWinEvent.Call(a.hHook)
		a.hHook = 0
	}
	if a.hMutex != 0 {
		procCloseHandle.Call(a.hMutex)
		a.hMutex = 0
	}
	procShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&a.nid)))
	procPostQuitMessage.Call(0)
}

func wndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	switch msg {
	case WM_QUERYENDSESSION:
		return 1

	case WM_ENDSESSION:
		if wParam != 0 {
			app.exit()
		}
		return 0

	case WM_ACTIVATE:
		if (wParam & 0xffff) != 0 {
			app.checkReloadConfig()
		}
		ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret

	case WM_CLOSE:
		procShowWindow.Call(hwnd, SW_HIDE)
		return 0

	case WM_TIMER:
		if wParam == IDT_FOCUS_TIMER {
			if app.config.Enabled {
				fg := getForegroundProcess()
				if fg != "" && fg != app.currentFg {
					app.onForegroundChanged(fg)
				}
			}
		}
		return 0

	case WM_TRAYICON:
		if lParam == WM_LBUTTONDBL || lParam == WM_LBUTTONUP {
			app.checkReloadConfig()
			procShowWindow.Call(hwnd, 9) // SW_RESTORE
			procSetForegroundWindow.Call(hwnd)
			app.refreshListBox()
		} else if lParam == WM_RBUTTONUP {
			app.checkReloadConfig()
			showTrayMenu(hwnd)
		}
		return 0

	case WM_COMMAND:
		id := int(wParam & 0xffff)
		switch id {
		case IDC_BTN_ADD:
			var buf [256]uint16
			procGetWindowText.Call(app.edtProcess, uintptr(unsafe.Pointer(&buf[0])), 256)
			name := cleanProcessName(syscall.UTF16ToString(buf[:]))
			if name != "" && !isIgnoredExternalApp(name) {
				app.managed[name] = true
				app.saveConfig()
				app.applyMuteState(app.config.Fade)
				app.refreshListBox()
				procSetWindowText.Call(app.edtProcess, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(""))))
			}

		case IDC_LST_MANAGED:
			notifyCode := (wParam >> 16) & 0xffff
			if notifyCode == 1 { // LBN_SELCHANGE
				idx, _, _ := procSendMessage.Call(app.lstManaged, LB_GETCURSEL, 0, 0)
				if int(idx) >= 0 {
					var buf [256]uint16
					procSendMessage.Call(app.lstManaged, LB_GETTEXT, idx, uintptr(unsafe.Pointer(&buf[0])))
					pName := syscall.UTF16ToString(buf[:])
					if pName != "" {
						procSetWindowText.Call(app.edtProcess, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(pName))))
					}
				}
			}

		case IDC_BTN_ADDFG:
			if app.externalFg != "" && !isIgnoredExternalApp(app.externalFg) {
				if app.managed[app.externalFg] {
					app.cancelFade(app.externalFg)
					delete(app.managed, app.externalFg)
					app.saveConfig()
					app.setProcessVolume(app.externalFg, 1.0)
					app.refreshListBox()
				} else {
					app.managed[app.externalFg] = true
					app.saveConfig()
					app.applyMuteState(app.config.Fade)
					app.refreshListBox()
				}
			}

		case IDC_BTN_BROWSE:
			exeName := browseExe(hwnd)
			if exeName != "" {
				procSetWindowText.Call(app.edtProcess, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(exeName))))
			}

		case IDC_BTN_REMOVE:
			idx, _, _ := procSendMessage.Call(app.lstManaged, LB_GETCURSEL, 0, 0)
			if int(idx) >= 0 {
				var buf [256]uint16
				procSendMessage.Call(app.lstManaged, LB_GETTEXT, idx, uintptr(unsafe.Pointer(&buf[0])))
				pName := syscall.UTF16ToString(buf[:])
				app.cancelFade(pName)
				delete(app.managed, pName)
				app.saveConfig()
				app.setProcessVolume(pName, 1.0)
				app.refreshListBox()
			}

		case IDC_BTN_CLEAR:
			app.restoreAllManaged()
			app.managed = make(map[string]bool)
			app.saveConfig()
			app.refreshListBox()

		case IDC_BTN_OPEN_CONFIG:
			cfgAbs, _ := filepath.Abs(app.configPath)
			if _, err := os.Stat(cfgAbs); os.IsNotExist(err) {
				app.saveConfig()
			}
			procShellExecute.Call(
				0,
				uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("open"))),
				uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("notepad.exe"))),
				uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(cfgAbs))),
				0,
				1,
			)

		case IDC_CHK_ENABLE:
			app.config.Enabled = !app.config.Enabled
			app.saveConfig()
			app.applyMuteState(app.config.Fade)
			app.refreshListBox()

		case IDC_CHK_FADE:
			app.config.Fade = !app.config.Fade
			if !app.config.Fade {
				app.cancelAllFades()
				app.applyMuteState(false)
			}
			app.saveConfig()
			app.refreshListBox()

		case IDC_CHK_AUTOSTART:
			cur := isAutostartEnabled()
			setAutostart(!cur)
			app.refreshListBox()

		case IDM_TRAY_SHOW:
			app.checkReloadConfig()
			procShowWindow.Call(hwnd, SW_SHOW)
			procSetForegroundWindow.Call(hwnd)
			app.refreshListBox()

		case IDM_TRAY_TOGGLE:
			app.config.Enabled = !app.config.Enabled
			app.saveConfig()
			app.applyMuteState(app.config.Fade)
			app.refreshListBox()

		case IDM_TRAY_FADE:
			app.config.Fade = !app.config.Fade
			if !app.config.Fade {
				app.cancelAllFades()
				app.applyMuteState(false)
			}
			app.saveConfig()
			app.refreshListBox()

		case IDM_TRAY_AUTOSTART:
			cur := isAutostartEnabled()
			setAutostart(!cur)
			app.refreshListBox()

		case IDM_TRAY_EXIT:
			app.exit()
			return 0
		}
		return 0

	case WM_DESTROY:
		app.exit()
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func showTrayMenu(hwnd uintptr) {
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	procAppendMenu.Call(hMenu, MF_STRING, uintptr(IDM_TRAY_SHOW), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("打开主界面"))))
	procAppendMenu.Call(hMenu, MF_SEPARATOR, 0, 0)

	toggleFlags := uintptr(MF_STRING)
	if app.config.Enabled {
		toggleFlags |= MF_CHECKED
	}
	procAppendMenu.Call(hMenu, toggleFlags, uintptr(IDM_TRAY_TOGGLE), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("启用后台静音"))))

	fadeFlags := uintptr(MF_STRING)
	if app.config.Fade {
		fadeFlags |= MF_CHECKED
	}
	procAppendMenu.Call(hMenu, fadeFlags, uintptr(IDM_TRAY_FADE), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("平滑淡入淡出 (1.0s)"))))

	autoFlags := uintptr(MF_STRING)
	if isAutostartEnabled() {
		autoFlags |= MF_CHECKED
	}
	procAppendMenu.Call(hMenu, autoFlags, uintptr(IDM_TRAY_AUTOSTART), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("开机自启动"))))

	procAppendMenu.Call(hMenu, MF_SEPARATOR, 0, 0)
	procAppendMenu.Call(hMenu, MF_STRING, uintptr(IDM_TRAY_EXIT), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("退出 AutoMute"))))

	procSetForegroundWindow.Call(hwnd)
	procTrackPopupMenu.Call(hMenu, TPM_BOTTOMALIGN|TPM_RIGHTALIGN, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procPostMessage.Call(hwnd, 0, 0, 0)
}

func loadAppIcon(exeDir string) uintptr {
	icoCandidates := []string{
		filepath.Join(exeDir, "app.ico"),
		"app.ico",
	}
	for _, ico := range icoCandidates {
		if _, err := os.Stat(ico); err == nil {
			absIco, _ := filepath.Abs(ico)
			hIcon, _, _ := procLoadImage.Call(
				0,
				uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(absIco))),
				IMAGE_ICON,
				0, 0,
				LR_LOADFROMFILE|LR_DEFAULTSIZE,
			)
			if hIcon != 0 {
				return hIcon
			}
		}
	}

	if len(embeddedIconBytes) > 22 {
		numImages := int(binary.LittleEndian.Uint16(embeddedIconBytes[4:6]))
		bestIdx := 0
		maxBytes := uint32(0)
		for i := 0; i < numImages; i++ {
			entryOffset := 6 + i*16
			if entryOffset+16 <= len(embeddedIconBytes) {
				cb := binary.LittleEndian.Uint32(embeddedIconBytes[entryOffset+8 : entryOffset+12])
				if cb > maxBytes {
					maxBytes = cb
					bestIdx = i
				}
			}
		}
		if maxBytes > 0 {
			entryOffset := 6 + bestIdx*16
			if entryOffset+16 <= len(embeddedIconBytes) {
				imgSize := binary.LittleEndian.Uint32(embeddedIconBytes[entryOffset+8 : entryOffset+12])
				imgOffset := binary.LittleEndian.Uint32(embeddedIconBytes[entryOffset+12 : entryOffset+16])
				if int(imgOffset+imgSize) <= len(embeddedIconBytes) {
					hIcon, _, _ := procCreateIconFromResourceEx.Call(
						uintptr(unsafe.Pointer(&embeddedIconBytes[imgOffset])),
						uintptr(imgSize),
						1,
						0x00030000,
						0, 0,
						LR_DEFAULTSIZE,
					)
					if hIcon != 0 {
						return hIcon
					}
				}
			}
		}
	}

	hIcon, _, _ := procLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	return hIcon
}

func createControl(className, text string, style uint32, x, y, w, h int, id int, parent, hInstance, hFont uintptr) uintptr {
	hCtrl, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(className))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE|style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	if hFont != 0 {
		procSendMessage.Call(hCtrl, WM_SETFONT, hFont, 1)
	}
	return hCtrl
}

func main() {
	// 1. Lock Main Goroutine to OS Thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 2. Initialize COM in STA (Single-Threaded Apartment)
	procCoInitialize.Call(0, 2)
	defer procCoUninitialize.Call()

	// 3. Single Instance Check
	className := syscall.StringToUTF16Ptr("AutoMuteMainWndClass")
	mutexName := syscall.StringToUTF16Ptr("Local\\AutoMute_SingleInstance_Mutex")
	hMutex, _, _ := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	lastErr, _, _ := procGetLastError.Call()
	if lastErr == ERROR_ALREADY_EXISTS {
		existingWnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
		if existingWnd != 0 && !shouldStartMinimized(os.Args[1:]) {
			procShowWindow.Call(existingWnd, 9) // SW_RESTORE
			procSetForegroundWindow.Call(existingWnd)
		}
		return
	}
	app.hMutex = hMutex

	// 4. Locate & Load Config
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	app.selfName = strings.ToLower(filepath.Base(exePath))
	configPath := filepath.Join(exeDir, "settings.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		altConfig := filepath.Join(cwd, "settings.json")
		if _, err2 := os.Stat(altConfig); err2 == nil {
			configPath = altConfig
		}
	}
	if absConfig, err := filepath.Abs(configPath); err == nil {
		configPath = absConfig
	}
	app.configPath = configPath
	app.loadConfig()

	// Ensure autostart command in registry contains --minimized for silent boot
	if isAutostartEnabled() {
		setAutostart(true)
	}

	// 5. Register Window Class
	hInstance, _, _ := procGetModuleHandle.Call(0)
	hIcon := loadAppIcon(exeDir)
	hCursor, _, _ := procLoadCursor.Call(0, uintptr(IDC_ARROW))

	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInstance,
		LpszClassName: className,
		HIcon:         hIcon,
		HIconSm:       hIcon,
		HCursor:       hCursor,
		HbrBackground: 15 + 1, // COLOR_BTNFACE + 1
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	// 6. Create Main Window (Task Manager style: 560x480, non-resizable)
	winWidth := 560
	winHeight := 480
	xPos := (1920 - winWidth) / 2
	yPos := (1080 - winHeight) / 2
	if xPos < 50 {
		xPos = 100
	}
	if yPos < 50 {
		yPos = 100
	}

	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("AutoMute - 后台静音管理器"))),
		WS_OVERLAPPEDWINDOW&^(WS_THICKFRAME|WS_MAXIMIZEBOX),
		uintptr(xPos), uintptr(yPos), uintptr(winWidth), uintptr(winHeight),
		0, 0, hInstance, 0,
	)
	app.hwnd = hwnd

	// Setup 100ms periodic focus verification fallback (R2)
	procSetTimer.Call(hwnd, IDT_FOCUS_TIMER, 100, 0)

	// Create Modern YaHei Font
	hFont, _, _ := procCreateFont.Call(
		15, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Microsoft YaHei UI"))),
	)
	if hFont == 0 {
		hFont, _, _ = procGetStockObject.Call(17)
	}

	// 7. Create UI Controls
	// Header Label
	createControl("STATIC", "已配置的后台自动静音进程列表 (白名单):", 0, 20, 15, 500, 20, 0, hwnd, hInstance, hFont)

	// Process ListBox
	app.lstManaged = createControl("LISTBOX", "", WS_BORDER|WS_VSCROLL|LBS_NOTIFY, 20, 38, 505, 175, IDC_LST_MANAGED, hwnd, hInstance, hFont)

	// Add Row 1: Manual Input + Browse
	createControl("STATIC", "添加进程:", 0, 20, 225, 70, 24, 0, hwnd, hInstance, hFont)
	app.edtProcess = createControl("EDIT", "", WS_BORDER|ES_AUTOHSCROLL, 95, 222, 215, 25, IDC_EDT_PROCESS, hwnd, hInstance, hFont)
	createControl("BUTTON", "浏览 exe...", BS_PUSHBUTTON, 320, 221, 105, 27, IDC_BTN_BROWSE, hwnd, hInstance, hFont)
	createControl("BUTTON", "+ 手动添加", BS_PUSHBUTTON, 435, 221, 90, 27, IDC_BTN_ADD, hwnd, hInstance, hFont)

	// Add Row 2: Quick Add Current/Recent Foreground App
	app.btnAddFg = createControl("BUTTON", "+ 一键添加前台应用", BS_PUSHBUTTON, 20, 258, 505, 32, IDC_BTN_ADDFG, hwnd, hInstance, hFont)

	// Action Buttons
	createControl("BUTTON", "- 移除选中的进程", BS_PUSHBUTTON, 20, 300, 140, 28, IDC_BTN_REMOVE, hwnd, hInstance, hFont)
	createControl("BUTTON", "清空列表", BS_PUSHBUTTON, 170, 300, 90, 28, IDC_BTN_CLEAR, hwnd, hInstance, hFont)
	createControl("BUTTON", "打开配置文件", BS_PUSHBUTTON, 270, 300, 110, 28, IDC_BTN_OPEN_CONFIG, hwnd, hInstance, hFont)

	// Settings Checkboxes (Row 4: 启用静音 + 平滑淡入淡出 + 开机自启动)
	app.chkEnable = createControl("BUTTON", "启用后台自动静音", BS_AUTOCHECKBOX, 20, 342, 150, 24, IDC_CHK_ENABLE, hwnd, hInstance, hFont)
	app.chkFade = createControl("BUTTON", "平滑淡入淡出 (1.0s)", BS_AUTOCHECKBOX, 180, 342, 170, 24, IDC_CHK_FADE, hwnd, hInstance, hFont)
	app.chkAuto = createControl("BUTTON", "开机自启动", BS_AUTOCHECKBOX, 360, 342, 120, 24, IDC_CHK_AUTOSTART, hwnd, hInstance, hFont)

	// Status & Tip
	app.lblStatus = createControl("STATIC", "状态: 初始化中...", 0, 20, 382, 505, 24, IDC_LBL_STATUS, hwnd, hInstance, hFont)
	createControl("STATIC", "提示: 点击右上角 [X] 关闭按钮将最小化到右下角托盘并在后台静默工作", 0, 20, 408, 505, 20, 0, hwnd, hInstance, hFont)

	// 8. Setup System Tray Icon
	app.nid = NOTIFYICONDATAW{
		CbSize:           uint32(unsafe.Sizeof(app.nid)),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            hIcon,
	}
	copy(app.nid.SzTip[:], syscall.StringToUTF16("AutoMute - 后台静音管理器 (双击打开)"))
	procShellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&app.nid)))

	// 9. Hook Windows EVENT_SYSTEM_FOREGROUND
	hookCallback := syscall.NewCallback(winEventProc)
	hHook, _, _ := procSetWinEventHook.Call(
		EVENT_SYSTEM_FOREGROUND,
		EVENT_SYSTEM_FOREGROUND,
		0,
		hookCallback,
		0, 0,
		WINEVENT_OUTOFCONTEXT|WINEVENT_SKIPOWNPROCESS,
	)
	app.hHook = hHook

	// Initial Foreground Check
	initFg := getForegroundProcess()
	app.currentFg = initFg
	if !isIgnoredExternalApp(initFg) {
		app.externalFg = initFg
	}
	app.applyMuteState(false)
	app.refreshListBox()

	// 10. Show Main Window immediately on startup (unless launched minimized/silent in tray)
	if !shouldStartMinimized(os.Args[1:]) {
		procShowWindow.Call(hwnd, SW_SHOW)
		procUpdateWindow.Call(hwnd)
		procSetForegroundWindow.Call(hwnd)
	}

	// 11. Win32 Single-Threaded Message Loop
	var msg MSG
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
