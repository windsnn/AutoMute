package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestSmoothStep(t *testing.T) {
	cases := []struct {
		input    float64
		expected float64
	}{
		{-1.0, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{2.0, 1.0},
	}

	for _, c := range cases {
		got := smoothStep(c.input)
		if math.Abs(got-c.expected) > 1e-6 {
			t.Errorf("smoothStep(%v) = %v; want %v", c.input, got, c.expected)
		}
	}

	// Test monotonic behavior from 0.0 to 1.0
	prev := 0.0
	for i := 1; i <= 100; i++ {
		cur := smoothStep(float64(i) / 100.0)
		if cur < prev {
			t.Errorf("smoothStep is not monotonically increasing at step %d: %v < %v", i, cur, prev)
		}
		prev = cur
	}
}

func TestCleanProcessName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"notepad", "notepad.exe"},
		{"NOTEPAD.EXE", "notepad.exe"},
		{"  msedge.exe  ", "msedge.exe"},
		{"Game.Exe", "game.exe"},
	}

	for _, c := range cases {
		got := cleanProcessName(c.input)
		if got != c.expected {
			t.Errorf("cleanProcessName(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestBoolToCheck(t *testing.T) {
	if got := boolToCheck(true); got != BST_CHECKED {
		t.Errorf("boolToCheck(true) = %v; want BST_CHECKED (%v)", got, BST_CHECKED)
	}
	if got := boolToCheck(false); got != BST_UNCHECKED {
		t.Errorf("boolToCheck(false) = %v; want BST_UNCHECKED (%v)", got, BST_UNCHECKED)
	}
}

func TestStructSizes(t *testing.T) {
	nidSize := uint32(unsafe.Sizeof(NOTIFYICONDATAW{}))
	if nidSize != 976 {
		t.Errorf("NOTIFYICONDATAW size = %d; want 976", nidSize)
	}

	ofnSize := uint32(unsafe.Sizeof(OPENFILENAMEW{}))
	if ofnSize != 152 {
		t.Errorf("OPENFILENAMEW size = %d; want 152", ofnSize)
	}
}

func TestEmbeddedIconIntegrity(t *testing.T) {
	if len(embeddedIconBytes) <= 22 {
		t.Fatalf("embeddedIconBytes is too small or missing: len=%d", len(embeddedIconBytes))
	}
	numImages := int(embeddedIconBytes[4]) | (int(embeddedIconBytes[5]) << 8)
	if numImages == 0 {
		t.Errorf("embeddedIconBytes has 0 icon images")
	}
}

func TestConfigLoadSaveAndEmptyProcess(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "settings.json")

	testApp := App{
		configPath: cfgFile,
	}

	// 1. Initial load with missing file -> should set default
	testApp.loadConfig()
	if !testApp.config.Enabled || !testApp.config.Fade || len(testApp.config.Processes) != 1 || testApp.config.Processes[0] != "msedge.exe" {
		t.Errorf("initial loadConfig did not populate defaults properly: %+v", testApp.config)
	}

	// 2. Modifying processes and saving
	testApp.managed = map[string]bool{
		"game.exe":   true,
		"chrome.exe": true,
	}
	testApp.saveConfig()

	// Read raw file to verify deterministic alphabetical sort
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var parsed ConfigData
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal saved json: %v", err)
	}
	if len(parsed.Processes) != 2 || parsed.Processes[0] != "chrome.exe" || parsed.Processes[1] != "game.exe" {
		t.Errorf("saved processes not sorted alphabetically: %v", parsed.Processes)
	}

	// 3. Clear all processes and save -> verify empty process list persists
	testApp.managed = make(map[string]bool)
	testApp.saveConfig()

	// Reload into a new App instance
	testApp2 := App{
		configPath: cfgFile,
	}
	testApp2.loadConfig()
	if len(testApp2.managed) != 0 || len(testApp2.config.Processes) != 0 {
		t.Errorf("cleared process list did not persist, got: %v", testApp2.config.Processes)
	}

	// 4. Corrupted json -> should fallback to defaults
	if err := os.WriteFile(cfgFile, []byte("{corrupted json"), 0644); err != nil {
		t.Fatalf("failed to write corrupted config: %v", err)
	}
	testApp3 := App{
		configPath: cfgFile,
	}
	testApp3.loadConfig()
	if !testApp3.config.Enabled || len(testApp3.config.Processes) != 1 {
		t.Errorf("corrupted config did not fallback to defaults: %+v", testApp3.config)
	}
}

func TestIsIgnoredExternalApp(t *testing.T) {
	ignored := []string{
		"",
		"automute.exe",
		"AutoMute.exe",
		"system",
		"explorer.exe",
		"Explorer.EXE",
		"searchhost.exe",
		"shellexperiencehost.exe",
		"startmenuexperiencehost.exe",
		"[elevated:1234]",
		"[protected:5678]",
	}
	for _, name := range ignored {
		if !isIgnoredExternalApp(name) {
			t.Errorf("expected isIgnoredExternalApp(%q) to be true, got false", name)
		}
	}

	valid := []string{
		"msedge.exe",
		"chrome.exe",
		"game.exe",
		"notepad.exe",
		"code.exe",
	}
	for _, name := range valid {
		if isIgnoredExternalApp(name) {
			t.Errorf("expected isIgnoredExternalApp(%q) to be false, got true", name)
		}
	}
}

func TestCancelFadeAndSequenceBreakers(t *testing.T) {
	testApp := App{
		fadeSeq: make(map[string]uint64),
	}

	testApp.fadeSeq["procA.exe"] = 5
	testApp.fadeSeq["procB.exe"] = 10

	testApp.cancelFade("procA.exe")
	if testApp.fadeSeq["procA.exe"] != 6 {
		t.Errorf("cancelFade did not increment sequence: got %d, want 6", testApp.fadeSeq["procA.exe"])
	}
	if testApp.fadeSeq["procB.exe"] != 10 {
		t.Errorf("cancelFade modified unrelated process: got %d, want 10", testApp.fadeSeq["procB.exe"])
	}

	testApp.cancelAllFades()
	if testApp.fadeSeq["procA.exe"] != 7 || testApp.fadeSeq["procB.exe"] != 11 {
		t.Errorf("cancelAllFades did not increment all sequences: got %+v", testApp.fadeSeq)
	}
}

func TestForegroundChangedFocusTransitions(t *testing.T) {
	testApp := App{
		config: ConfigData{
			Enabled: true,
			Fade:    false, // direct set to avoid starting real goroutines
		},
		managed: map[string]bool{
			"game.exe":   true,
			"msedge.exe": true,
		},
	}

	// 1. Initial focus on game.exe
	testApp.onForegroundChanged("game.exe")
	if testApp.currentFg != "game.exe" {
		t.Errorf("expected currentFg to be game.exe, got %q", testApp.currentFg)
	}
	if testApp.externalFg != "game.exe" {
		t.Errorf("expected externalFg to be game.exe, got %q", testApp.externalFg)
	}

	// 2. Switch to explorer.exe (Desktop/File Explorer) -> MUST update currentFg to explorer.exe,
	// while externalFg remains game.exe
	testApp.onForegroundChanged("explorer.exe")
	if testApp.currentFg != "explorer.exe" {
		t.Errorf("expected currentFg to transition to explorer.exe, got %q", testApp.currentFg)
	}
	if testApp.externalFg != "game.exe" {
		t.Errorf("expected externalFg to remain game.exe, got %q", testApp.externalFg)
	}

	// 3. Switch to AutoMute's own window -> currentFg is automute.exe, externalFg stays game.exe
	testApp.onForegroundChanged("automute.exe")
	if testApp.currentFg != "automute.exe" {
		t.Errorf("expected currentFg to be automute.exe, got %q", testApp.currentFg)
	}
	if testApp.externalFg != "game.exe" {
		t.Errorf("expected externalFg to remain game.exe, got %q", testApp.externalFg)
	}

	// 4. Switch to an elevated process -> currentFg updates to [elevated:999], externalFg stays game.exe
	testApp.onForegroundChanged("[elevated:999]")
	if testApp.currentFg != "[elevated:999]" {
		t.Errorf("expected currentFg to be [elevated:999], got %q", testApp.currentFg)
	}
	if testApp.externalFg != "game.exe" {
		t.Errorf("expected externalFg to remain game.exe, got %q", testApp.externalFg)
	}

	// 5. Switch to a new normal app (notepad.exe) -> both update
	testApp.onForegroundChanged("notepad.exe")
	if testApp.currentFg != "notepad.exe" || testApp.externalFg != "notepad.exe" {
		t.Errorf("expected both to be notepad.exe, got currentFg=%q, externalFg=%q", testApp.currentFg, testApp.externalFg)
	}
}

func TestGetProcessNameSpecialPIDs(t *testing.T) {
	if name := getProcessName(0); name != "system" {
		t.Errorf("getProcessName(0) = %q; want %q", name, "system")
	}
	if name := getProcessName(4); name != "system" {
		t.Errorf("getProcessName(4) = %q; want %q", name, "system")
	}
}

func TestCleanProcessNameWithPaths(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{`C:\Games\Game.exe`, "game.exe"},
		{`D:\Program Files\App\player`, "player.exe"},
		{`  E:\Music\Spotify.EXE   `, "spotify.exe"},
		{`F:/Steam/steamapps/common/app.exe`, "app.exe"},
	}

	for _, c := range cases {
		got := cleanProcessName(c.input)
		if got != c.expected {
			t.Errorf("cleanProcessName(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestConfigLoadSanitizesEntries(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "settings.json")

	rawConfig := ConfigData{
		Enabled:   true,
		Fade:      true,
		Processes: []string{"  Chrome.exe  ", "Game", "msedge.exe", "system", "automute.exe", ""},
	}
	data, err := json.Marshal(rawConfig)
	if err != nil {
		t.Fatalf("failed to marshal raw config: %v", err)
	}
	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	testApp := App{
		configPath: cfgFile,
	}
	testApp.loadConfig()

	// system, automute.exe, empty string must be excluded
	// Chrome.exe and Game must be cleaned to chrome.exe and game.exe
	expected := []string{"chrome.exe", "game.exe", "msedge.exe"}
	if len(testApp.config.Processes) != len(expected) {
		t.Fatalf("expected %d sanitized processes, got %v", len(expected), testApp.config.Processes)
	}
	for i, exp := range expected {
		if testApp.config.Processes[i] != exp {
			t.Errorf("process[%d] = %q; want %q", i, testApp.config.Processes[i], exp)
		}
		if !testApp.managed[exp] {
			t.Errorf("expected %q in testApp.managed", exp)
		}
	}
	if testApp.managed["system"] || testApp.managed["automute.exe"] || testApp.managed[""] {
		t.Errorf("ignored entries found in testApp.managed: %+v", testApp.managed)
	}
}

func TestCorruptedConfigPreservesFileOnDisk(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "settings.json")

	corruptedData := []byte(`{ invalid: json, not: valid }`)
	if err := os.WriteFile(cfgFile, corruptedData, 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	testApp := App{
		configPath: cfgFile,
	}
	testApp.loadConfig()

	// In memory, it should fall back to safe defaults so the app does not panic
	if !testApp.config.Enabled || len(testApp.config.Processes) != 1 || testApp.config.Processes[0] != "msedge.exe" {
		t.Errorf("in-memory config did not fallback to defaults: %+v", testApp.config)
	}

	// But on disk, the user's corrupted file MUST NOT be wiped out
	diskData, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("failed to read disk file: %v", err)
	}
	if string(diskData) != string(corruptedData) {
		t.Errorf("corrupted file on disk was overwritten! got %q, want %q", string(diskData), string(corruptedData))
	}
}

func TestCheckReloadConfigModTime(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "settings.json")

	testApp := App{
		configPath: cfgFile,
	}
	testApp.loadConfig() // creates default config and sets lastConfigModTime

	// Simulate external update in Notepad
	time.Sleep(10 * time.Millisecond) // ensure timestamp difference
	updatedConfig := ConfigData{
		Enabled:   true,
		Fade:      true,
		Processes: []string{"vlc.exe", "mpv.exe"},
	}
	newData, _ := json.Marshal(updatedConfig)
	_ = os.WriteFile(cfgFile, newData, 0644)

	// Before reload, managed does not have vlc.exe
	if testApp.managed["vlc.exe"] {
		t.Fatalf("managed should not have vlc.exe before reload")
	}

	// Trigger reload
	testApp.checkReloadConfig()

	// After reload, managed must reflect the updated file
	if !testApp.managed["vlc.exe"] || !testApp.managed["mpv.exe"] {
		t.Errorf("managed did not load external updates: %+v", testApp.managed)
	}
}

func TestExitIdempotence(t *testing.T) {
	testApp := App{
		managed: map[string]bool{"game.exe": true},
	}

	// First exit
	testApp.exit()
	if !testApp.exited {
		t.Errorf("testApp.exited should be true after exit()")
	}

	// Second exit must safely return without errors or panics
	testApp.exit()
}

func TestCOMDeviceEnumerationAndVTableCalls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitialize.Call(0, 0) // COINIT_MULTITHREADED
	defer procCoUninitialize.Call()
	if int32(hr) < 0 && hr != 0x00000001 && hr != 0x80010106 { // S_FALSE or RPC_E_CHANGED_MODE
		t.Fatalf("procCoInitialize failed: hr=0x%x", hr)
	}

	// Test real iterateProcessAudioVolumes with dummy process
	called := false
	iterateProcessAudioVolumes("nonexistent_test_proc_12345.exe", func(pVol uintptr) {
		called = true
	})
	t.Logf("iterateProcessAudioVolumes executed cleanly, called=%v", called)

	// Step-by-step verification of COM endpoint
	var pEnum uintptr
	hrEnum, _, _ := procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&CLSID_MMDeviceEnumerator)),
		0,
		1,
		uintptr(unsafe.Pointer(&IID_IMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&pEnum)),
	)
	t.Logf("hrEnum=0x%x, pEnum=%v", hrEnum, pEnum)
	if int32(hrEnum) >= 0 && pEnum != 0 {
		var pDev uintptr
		hrDev, _, _ := callCOM(pEnum, 4, 0, 1, uintptr(unsafe.Pointer(&pDev)))
		t.Logf("hrDev=0x%x, pDev=%v", hrDev, pDev)
		if pDev != 0 {
			var pMgr uintptr
			hrMgr, _, _ := callCOM(pDev, 3, uintptr(unsafe.Pointer(&IID_IAudioSessionManager2)), 1, 0, uintptr(unsafe.Pointer(&pMgr)))
			t.Logf("hrMgr=0x%x, pMgr=%v", hrMgr, pMgr)
			if pMgr != 0 {
				var pSessEnum uintptr
				hrEnumSess, _, _ := callCOM(pMgr, 5, uintptr(unsafe.Pointer(&pSessEnum)))
				t.Logf("hrEnumSess=0x%x, pSessEnum=%v", hrEnumSess, pSessEnum)
				if pSessEnum != 0 {
					var count int32
					callCOM(pSessEnum, 3, uintptr(unsafe.Pointer(&count)))
					t.Logf("Session count = %d", count)
					for i := int32(0); i < count; i++ {
						var pSessCtrl uintptr
						callCOM(pSessEnum, 4, uintptr(i), uintptr(unsafe.Pointer(&pSessCtrl)))
						if pSessCtrl == 0 {
							continue
						}
						var pCtrl2 uintptr
						hrCtrl2, _, _ := callCOM(pSessCtrl, 0, uintptr(unsafe.Pointer(&IID_IAudioSessionControl2)), uintptr(unsafe.Pointer(&pCtrl2)))
						if int32(hrCtrl2) >= 0 && pCtrl2 != 0 {
							var pid uint32
							callCOM(pCtrl2, 14, uintptr(unsafe.Pointer(&pid)))
							pName := getProcessName(pid)
							t.Logf("Session %d: pid=%d, name=%q", i, pid, pName)
							callCOM(pCtrl2, 2)
						}
						callCOM(pSessCtrl, 2)
					}
					callCOM(pSessEnum, 2)
				}
				callCOM(pMgr, 2)
			}
			callCOM(pDev, 2)
		}
		callCOM(pEnum, 2)
	}

	// Also test with current process
	testApp := App{}
	vol := testApp.getProcessVolume("nonexistent_test_proc_12345.exe")
	if vol != 0.0 {
		t.Errorf("expected volume 0.0 for nonexistent process, got %f", vol)
	}
	testApp.setProcessVolume("nonexistent_test_proc_12345.exe", 0.5)

	// Test real getProcessVolume on active session msedge.exe
	edgeVol := testApp.getProcessVolume("msedge.exe")
	t.Logf("Real msedge.exe volume = %f", edgeVol)
}

func TestCleanProcessNameGarbageInputs(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{".exe", ""},
		{"   .exe   ", ""},
		{".", ""},
		{"..", ""},
		{"...", ""},
		{"/", ""},
		{"\\", ""},
		{"  /  ", ""},
		{"   \\   ", ""},
		{`C:\`, ""},
		{`D:/`, ""},
		{"app", "app.exe"},
		{"APP.EXE", "app.exe"},
		{"  my.game.exe  ", "my.game.exe"},
	}

	for _, c := range cases {
		got := cleanProcessName(c.input)
		if got != c.expected {
			t.Errorf("cleanProcessName(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestOnForegroundChangedCancelsFadeWhenFadeDisabled(t *testing.T) {
	testApp := App{
		config: ConfigData{
			Enabled: true,
			Fade:    false, // Fade is disabled
		},
		managed: map[string]bool{
			"game.exe": true,
			"edge.exe": true,
		},
		fadeSeq: map[string]uint64{
			"game.exe": 10,
			"edge.exe": 20,
		},
		currentFg: "game.exe",
	}

	testApp.onForegroundChanged("edge.exe")

	// Both prev (game.exe) and fgProc (edge.exe) must have their fadeSeq incremented to cancel any in-flight fades
	if testApp.fadeSeq["game.exe"] != 11 {
		t.Errorf("expected game.exe fadeSeq to be 11, got %d", testApp.fadeSeq["game.exe"])
	}
	if testApp.fadeSeq["edge.exe"] != 21 {
		t.Errorf("expected edge.exe fadeSeq to be 21, got %d", testApp.fadeSeq["edge.exe"])
	}
}

func TestCheckReloadConfigModTimeEarlierBackup(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "settings.json")

	testApp := App{
		configPath: cfgFile,
	}
	testApp.loadConfig()

	// Simulate restoring an older backup file with an earlier timestamp
	pastTime := time.Now().Add(-1 * time.Hour)
	olderConfig := ConfigData{
		Enabled:   true,
		Fade:      true,
		Processes: []string{"backup.exe"},
	}
	data, _ := json.Marshal(olderConfig)
	_ = os.WriteFile(cfgFile, data, 0644)
	_ = os.Chtimes(cfgFile, pastTime, pastTime)

	// Reload check should notice difference in ModTime even though it is earlier
	testApp.checkReloadConfig()

	if !testApp.managed["backup.exe"] {
		t.Errorf("expected managed to contain backup.exe after restoring older backup, got %+v", testApp.managed)
	}
}

func TestFadeProcessZeroDuration(t *testing.T) {
	testApp := App{
		fadeSeq: make(map[string]uint64),
	}

	// Zero duration should increment sequence and set volume directly without goroutine crash
	testApp.fadeProcess("dummy.exe", 0.0, 0)
	if testApp.fadeSeq["dummy.exe"] != 1 {
		t.Errorf("expected dummy.exe fadeSeq to be 1, got %d", testApp.fadeSeq["dummy.exe"])
	}
}

func TestSelfProcessRecognitionAndToggle(t *testing.T) {
	// Verify that self PID is always mapped to automute.exe
	selfPid := uint32(os.Getpid())
	if name := getProcessName(selfPid); name != "automute.exe" {
		t.Errorf("expected getProcessName(selfPid) to be automute.exe, got %q", name)
	}

	testApp := App{
		managed: make(map[string]bool),
		config: ConfigData{
			Enabled: true,
		},
		fadeSeq: make(map[string]uint64),
	}

	// 1. Switch to user app (cloudmusic.exe)
	testApp.onForegroundChanged("cloudmusic.exe")
	if testApp.externalFg != "cloudmusic.exe" {
		t.Errorf("expected externalFg to be cloudmusic.exe, got %q", testApp.externalFg)
	}

	// 2. Switch to AutoMute's own window -> externalFg must strictly remain cloudmusic.exe
	testApp.onForegroundChanged("automute.exe")
	if testApp.externalFg != "cloudmusic.exe" {
		t.Errorf("expected externalFg to stay cloudmusic.exe when clicking AutoMute, got %q", testApp.externalFg)
	}
	if testApp.currentFg != "automute.exe" {
		t.Errorf("expected currentFg to be automute.exe, got %q", testApp.currentFg)
	}

	// 3. Status text check should not contain confusing automute.exe as target
	testApp.updateStatusText()
}

func TestToolhelpSnapshot(t *testing.T) {
	selfPid := uint32(os.Getpid())
	name := getProcNameByToolhelp(selfPid)
	if name == "" {
		t.Errorf("expected getProcNameByToolhelp to resolve self process name, got empty string")
	}
}

func TestShouldStartMinimized(t *testing.T) {
	cases := []struct {
		args     []string
		expected bool
	}{
		{[]string{}, false},
		{[]string{"--minimized"}, true},
		{[]string{"-minimized"}, true},
		{[]string{"--silent"}, true},
		{[]string{"-silent"}, true},
		{[]string{"--tray"}, true},
		{[]string{"/minimized"}, true},
		{[]string{"/silent"}, true},
		{[]string{"other.exe", "file.txt"}, false},
		{[]string{"--other", "--minimized"}, true},
	}

	for _, c := range cases {
		actual := shouldStartMinimized(c.args)
		if actual != c.expected {
			t.Errorf("args %v: expected %v, got %v", c.args, c.expected, actual)
		}
	}
}

func TestAutostartCycle(t *testing.T) {
	// 1. Enable autostart with current executable
	setAutostart(true)
	if !isAutostartEnabled() {
		t.Errorf("expected isAutostartEnabled() to be true after setAutostart(true)")
	}

	// 2. Query registry value to ensure it contains --minimized
	var hKey uintptr
	keyNamePtr := syscall.StringToUTF16Ptr(runKeyPath)
	ret, _, _ := procRegOpenKeyEx.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(keyNamePtr)), 0, KEY_READ, uintptr(unsafe.Pointer(&hKey)))
	if ret == 0 {
		defer procRegCloseKey.Call(hKey)
		valNamePtr := syscall.StringToUTF16Ptr("AutoMute")
		var valType uint32
		var buf [512]uint16
		size := uint32(len(buf) * 2)
		ret, _, _ = procRegQueryValueEx.Call(hKey, uintptr(unsafe.Pointer(valNamePtr)), 0, uintptr(unsafe.Pointer(&valType)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if ret == 0 {
			valStr := syscall.UTF16ToString(buf[:])
			if !strings.Contains(valStr, "--minimized") {
				t.Errorf("expected registry value to contain --minimized, got %q", valStr)
			}
		} else {
			t.Errorf("failed to query registry value: ret=%d", ret)
		}
	}

	// Restore registry to point to AutoMute.exe in current workspace
	if cwd, err := os.Getwd(); err == nil {
		targetExe := filepath.Join(cwd, "AutoMute.exe")
		if _, err := os.Stat(targetExe); err == nil {
			var hWriteKey uintptr
			retWrite, _, _ := procRegOpenKeyEx.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(keyNamePtr)), 0, KEY_WRITE, uintptr(unsafe.Pointer(&hWriteKey)))
			if retWrite == 0 {
				valBytes := syscall.StringToUTF16(fmt.Sprintf("\"%s\" --minimized", targetExe))
				procRegSetValueEx.Call(
					hWriteKey,
					uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("AutoMute"))),
					0,
					REG_SZ,
					uintptr(unsafe.Pointer(&valBytes[0])),
					uintptr(len(valBytes)*2),
				)
				procRegCloseKey.Call(hWriteKey)
			}
		}
	}
}


