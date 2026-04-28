package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	mrand "math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

const maxIterations = 500000

var (
	screenSizes  = []int{3000, 4000, 3120, 4160}
	coreCounts   = []int{8, 16, 24, 32}
	navigatorKey = []string{
		"registerProtocolHandler\u2212function registerProtocolHandler() { [native code] }",
		"storage\u2212[object StorageManager]",
		"locks\u2212[object LockManager]",
		"appCodeName\u2212Mozilla",
		"permissions\u2212[object Permissions]",
		"share\u2212function share() { [native code] }",
		"webdriver\u2212false",
		"managed\u2212[object NavigatorManagedData]",
		"canShare\u2212function canShare() { [native code] }",
		"vendor\u2212Google Inc.",
		"vendor\u2212Google Inc.",
		"mediaDevices\u2212[object MediaDevices]",
		"vibrate\u2212function vibrate() { [native code] }",
		"storageBuckets\u2212[object StorageBucketManager]",
		"mediaCapabilities\u2212[object MediaCapabilities]",
		"getGamepads\u2212function getGamepads() { [native code] }",
		"bluetooth\u2212[object Bluetooth]",
		"share\u2212function share() { [native code] }",
		"cookieEnabled\u2212true",
		"virtualKeyboard\u2212[object VirtualKeyboard]",
		"product\u2212Gecko",
		"mediaDevices\u2212[object MediaDevices]",
		"canShare\u2212function canShare() { [native code] }",
		"getGamepads\u2212function getGamepads() { [native code] }",
		"product\u2212Gecko",
		"xr\u2212[object XRSystem]",
		"clipboard\u2212[object Clipboard]",
		"storageBuckets\u2212[object StorageBucketManager]",
		"unregisterProtocolHandler\u2212function unregisterProtocolHandler() { [native code] }",
		"productSub\u221220030107",
		"login\u2212[object NavigatorLogin]",
		"vendorSub\u2212",
		"login\u2212[object NavigatorLogin]",
		"getInstalledRelatedApps\u2212function getInstalledRelatedApps() { [native code] }",
		"mediaDevices\u2212[object MediaDevices]",
		"locks\u2212[object LockManager]",
		"webkitGetUserMedia\u2212function webkitGetUserMedia() { [native code] }",
		"vendor\u2212Google Inc.",
		"xr\u2212[object XRSystem]",
		"mediaDevices\u2212[object MediaDevices]",
		"virtualKeyboard\u2212[object VirtualKeyboard]",
		"virtualKeyboard\u2212[object VirtualKeyboard]",
		"appName\u2212Netscape",
		"storageBuckets\u2212[object StorageBucketManager]",
		"presentation\u2212[object Presentation]",
		"onLine\u2212true",
		"mimeTypes\u2212[object MimeTypeArray]",
		"credentials\u2212[object CredentialsContainer]",
		"presentation\u2212[object Presentation]",
		"getGamepads\u2212function getGamepads() { [native code] }",
		"vendorSub\u2212",
		"virtualKeyboard\u2212[object VirtualKeyboard]",
		"serviceWorker\u2212[object ServiceWorkerContainer]",
		"xr\u2212[object XRSystem]",
		"product\u2212Gecko",
		"keyboard\u2212[object Keyboard]",
		"gpu\u2212[object GPU]",
		"getInstalledRelatedApps\u2212function getInstalledRelatedApps() { [native code] }",
		"webkitPersistentStorage\u2212[object DeprecatedStorageQuota]",
		"doNotTrack",
		"clearAppBadge\u2212function clearAppBadge() { [native code] }",
		"presentation\u2212[object Presentation]",
		"serial\u2212[object Serial]",
		"locks\u2212[object LockManager]",
		"requestMIDIAccess\u2212function requestMIDIAccess() { [native code] }",
		"locks\u2212[object LockManager]",
		"requestMediaKeySystemAccess\u2212function requestMediaKeySystemAccess() { [native code] }",
		"vendor\u2212Google Inc.",
		"pdfViewerEnabled\u2212true",
		"language\u2212zh-CN",
		"setAppBadge\u2212function setAppBadge() { [native code] }",
		"geolocation\u2212[object Geolocation]",
		"userAgentData\u2212[object NavigatorUAData]",
		"mediaCapabilities\u2212[object MediaCapabilities]",
		"requestMIDIAccess\u2212function requestMIDIAccess() { [native code] }",
		"getUserMedia\u2212function getUserMedia() { [native code] }",
		"mediaDevices\u2212[object MediaDevices]",
		"webkitPersistentStorage\u2212[object DeprecatedStorageQuota]",
		"sendBeacon\u2212function sendBeacon() { [native code] }",
		"hardwareConcurrency\u221232",
		"credentials\u2212[object CredentialsContainer]",
		"storage\u2212[object StorageManager]",
		"cookieEnabled\u2212true",
		"pdfViewerEnabled\u2212true",
		"windowControlsOverlay\u2212[object WindowControlsOverlay]",
		"scheduling\u2212[object Scheduling]",
		"pdfViewerEnabled\u2212true",
		"hardwareConcurrency\u221232",
		"xr\u2212[object XRSystem]",
		"webdriver\u2212false",
		"getInstalledRelatedApps\u2212function getInstalledRelatedApps() { [native code] }",
		"getInstalledRelatedApps\u2212function getInstalledRelatedApps() { [native code] }",
		"bluetooth\u2212[object Bluetooth]",
	}
	documentKey = []string{
		"_reactListeningo743lnnpvdg",
		"location",
	}
	windowKey = []string{
		"0", "window", "self", "document", "name", "location", "customElements",
		"history", "navigation", "locationbar", "menubar", "personalbar",
		"scrollbars", "statusbar", "toolbar", "status", "closed", "frames",
		"length", "top", "opener", "parent", "frameElement", "navigator",
		"origin", "external", "screen", "innerWidth", "innerHeight", "scrollX",
		"pageXOffset", "scrollY", "pageYOffset", "visualViewport", "screenX",
		"screenY", "outerWidth", "outerHeight", "devicePixelRatio",
		"clientInformation", "screenLeft", "screenTop", "styleMedia", "onsearch",
		"isSecureContext", "trustedTypes", "performance", "onappinstalled",
		"onbeforeinstallprompt", "crypto", "indexedDB", "sessionStorage",
		"localStorage", "onbeforexrselect", "onabort", "onbeforeinput",
		"onbeforematch", "onbeforetoggle", "onblur", "oncancel", "oncanplay",
		"oncanplaythrough", "onchange", "onclick", "onclose",
		"oncontentvisibilityautostatechange", "oncontextlost", "oncontextmenu",
		"oncontextrestored", "oncuechange", "ondblclick", "ondrag", "ondragend",
		"ondragenter", "ondragleave", "ondragover", "ondragstart", "ondrop",
		"ondurationchange", "onemptied", "onended", "onerror", "onfocus",
		"onformdata", "oninput", "oninvalid", "onkeydown", "onkeypress",
		"onkeyup", "onload", "onloadeddata", "onloadedmetadata", "onloadstart",
		"onmousedown", "onmouseenter", "onmouseleave", "onmousemove",
		"onmouseout", "onmouseover", "onmouseup", "onmousewheel", "onpause",
		"onplay", "onplaying", "onprogress", "onratechange", "onreset",
		"onresize", "onscroll", "onsecuritypolicyviolation", "onseeked",
		"onseeking", "onselect", "onslotchange", "onstalled", "onsubmit",
		"onsuspend", "ontimeupdate", "ontoggle", "onvolumechange", "onwaiting",
		"onwebkitanimationend", "onwebkitanimationiteration",
		"onwebkitanimationstart", "onwebkittransitionend", "onwheel",
		"onauxclick", "ongotpointercapture", "onlostpointercapture",
		"onpointerdown", "onpointermove", "onpointerrawupdate", "onpointerup",
		"onpointercancel", "onpointerover", "onpointerout", "onpointerenter",
		"onpointerleave", "onselectstart", "onselectionchange",
		"onanimationend", "onanimationiteration", "onanimationstart",
		"ontransitionrun", "ontransitionstart", "ontransitionend",
		"ontransitioncancel", "onafterprint", "onbeforeprint", "onbeforeunload",
		"onhashchange", "onlanguagechange", "onmessage", "onmessageerror",
		"onoffline", "ononline", "onpagehide", "onpageshow", "onpopstate",
		"onrejectionhandled", "onstorage", "onunhandledrejection", "onunload",
		"crossOriginIsolated", "scheduler", "alert", "atob", "blur", "btoa",
		"cancelAnimationFrame", "cancelIdleCallback", "captureEvents",
		"clearInterval", "clearTimeout", "close", "confirm", "createImageBitmap",
		"fetch", "find", "focus", "getComputedStyle", "getSelection",
		"matchMedia", "moveBy", "moveTo", "open", "postMessage", "print",
		"prompt", "queueMicrotask", "releaseEvents", "reportError",
		"requestAnimationFrame", "requestIdleCallback", "resizeBy", "resizeTo",
		"scroll", "scrollBy", "scrollTo", "setInterval", "setTimeout", "stop",
		"structuredClone", "webkitCancelAnimationFrame",
		"webkitRequestAnimationFrame", "chrome", "caches", "cookieStore",
		"ondevicemotion", "ondeviceorientation", "ondeviceorientationabsolute",
		"launchQueue", "documentPictureInPicture", "getScreenDetails",
		"queryLocalFonts", "showDirectoryPicker", "showOpenFilePicker",
		"showSaveFilePicker", "originAgentCluster", "onpageswap",
		"onpagereveal", "credentialless", "speechSynthesis", "onscrollend",
		"webkitRequestFileSystem", "webkitResolveLocalFileSystemURL",
		"sendMsgToSolverCS", "webpackChunk_N_E", "__next_set_public_path__",
		"next", "__NEXT_DATA__", "__SSG_MANIFEST_CB", "__NEXT_P", "_N_E",
		"regeneratorRuntime", "__REACT_INTL_CONTEXT__", "DD_RUM", "_",
		"filterCSS", "filterXSS", "__SEGMENT_INSPECTOR__", "__NEXT_PRELOADREADY",
		"Intercom", "__MIDDLEWARE_MATCHERS", "__STATSIG_SDK__",
		"__STATSIG_JS_SDK__", "__STATSIG_RERENDER_OVERRIDE__",
		"_oaiHandleSessionExpired", "__BUILD_MANIFEST", "__SSG_MANIFEST",
		"__intercomAssignLocation", "__intercomReloadLocation",
	}
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

func getParseTime() string {
	loc := loadLocationOrFixed(parseTimeZoneName, parseTimeFallbackOffsetSecs)
	return formatBrowserParseTime(time.Now(), loc)
}

func buildConfig(userAgent string) []any {
	now := time.Now()
	perfCounter := float64(now.UnixMilli()%1000000) + mrand.Float64()
	epochOffset := float64(now.UnixMilli()) - perfCounter

	return []any{
		screenSizes[mrand.IntN(len(screenSizes))], // 0: screen size
		getParseTime(), // 1: parse time
		4294705152,     // 2: constant
		0,              // 3: nonce_i (dynamic)
		userAgent,      // 4: user agent
		"https://chatgpt.com/backend-api/sentinel/sdk.js", // 5: script url
		"",                  // 6: dpl
		"en-US",             // 7
		"en-US,es-US,en,es", // 8
		0,                   // 9: nonce_j (dynamic)
		navigatorKey[mrand.IntN(len(navigatorKey))], // 10
		documentKey[mrand.IntN(len(documentKey))],   // 11
		windowKey[mrand.IntN(len(windowKey))],       // 12
		perfCounter,                                 // 13
		uuid.NewString(),                            // 14
		"",                                          // 15
		coreCounts[mrand.IntN(len(coreCounts))],     // 16
		epochOffset,                                 // 17
	}
}

// solvePoW solves the proof-of-work challenge.
// Returns the proof token string (prefixed with "gAAAAAB").
func solvePoW(seed, difficulty string) (string, error) {
	return solvePoWForUserAgent(seed, difficulty, defaultUserAgent)
}

func solvePoWForUserAgent(seed, difficulty, userAgent string) (string, error) {
	config := buildConfig(resolveUserAgent(userAgent))

	diffBytes, err := hex.DecodeString(difficulty)
	if err != nil {
		return "", fmt.Errorf("invalid difficulty hex %q: %w", difficulty, err)
	}
	diffLen := len(diffBytes)

	// Pre-build the 3 static JSON fragments around indices [3] and [9].
	// config[0:3], config[4:9], config[10:]
	part1JSON, _ := json.Marshal(config[:3])
	part4to8JSON, _ := json.Marshal(config[4:9])
	part10JSON, _ := json.Marshal(config[10:])

	// part1: "[val0,val1,val2," (drop trailing ']', add ',')
	staticPart1 := append(part1JSON[:len(part1JSON)-1], ',')
	// part2: ",val4,val5,val6,val7,val8," (drop leading '[' and trailing ']', wrap with commas)
	mid := part4to8JSON[1 : len(part4to8JSON)-1]
	staticPart2 := make([]byte, 0, len(mid)+2)
	staticPart2 = append(staticPart2, ',')
	staticPart2 = append(staticPart2, mid...)
	staticPart2 = append(staticPart2, ',')
	// part3: ",val10,...,val17]" (drop leading '[', prepend ',')
	tail := part10JSON[1:]
	staticPart3 := make([]byte, 0, len(tail)+1)
	staticPart3 = append(staticPart3, ',')
	staticPart3 = append(staticPart3, tail...)

	seedBytes := []byte(seed)

	for i := 0; i < maxIterations; i++ {
		iStr := []byte(fmt.Sprintf("%d", i))
		jStr := []byte(fmt.Sprintf("%d", i>>1))

		// Assemble: staticPart1 + i + staticPart2 + j + staticPart3
		assembled := make([]byte, 0, len(staticPart1)+len(iStr)+len(staticPart2)+len(jStr)+len(staticPart3))
		assembled = append(assembled, staticPart1...)
		assembled = append(assembled, iStr...)
		assembled = append(assembled, staticPart2...)
		assembled = append(assembled, jStr...)
		assembled = append(assembled, staticPart3...)

		// Base64 encode the assembled JSON
		b64 := base64.StdEncoding.EncodeToString(assembled)

		// SHA3-512 of seed + base64
		hasher := sha3.New512()
		hasher.Write(seedBytes)
		hasher.Write([]byte(b64))
		hash := hasher.Sum(nil)

		// Compare first diffLen bytes
		if bytesLE(hash[:diffLen], diffBytes) {
			return "gAAAAAB" + b64, nil
		}
	}

	// Fallback token
	fallback := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%q", seed)))
	return "gAAAAABwQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + fallback, nil
}

// bytesLE returns true if a <= b (lexicographic byte comparison).
func bytesLE(a, b []byte) bool {
	for i := range a {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true // equal
}

// generateRequirementsToken creates the "p" token for the chat-requirements request.
// Uses an easy difficulty "0fffff" with a random seed.
func generateRequirementsToken() string {
	return generateRequirementsTokenForUserAgent(defaultUserAgent)
}

func generateRequirementsTokenForUserAgent(userAgent string) string {
	config := buildConfig(resolveUserAgent(userAgent))
	seed := randomFloat()

	// Use easy difficulty — just solve with one iteration
	part1JSON, _ := json.Marshal(config[:3])
	part4to8JSON, _ := json.Marshal(config[4:9])
	part10JSON, _ := json.Marshal(config[10:])

	staticPart1 := append(part1JSON[:len(part1JSON)-1], ',')
	mid := part4to8JSON[1 : len(part4to8JSON)-1]
	staticPart2 := make([]byte, 0, len(mid)+2)
	staticPart2 = append(staticPart2, ',')
	staticPart2 = append(staticPart2, mid...)
	staticPart2 = append(staticPart2, ',')
	tail := part10JSON[1:]
	staticPart3 := make([]byte, 0, len(tail)+1)
	staticPart3 = append(staticPart3, ',')
	staticPart3 = append(staticPart3, tail...)

	_ = seed
	// config[3] = 0, config[9] = 0 for requirements token
	assembled := make([]byte, 0, len(staticPart1)+1+len(staticPart2)+1+len(staticPart3))
	assembled = append(assembled, staticPart1...)
	assembled = append(assembled, '0')
	assembled = append(assembled, staticPart2...)
	assembled = append(assembled, '0')
	assembled = append(assembled, staticPart3...)

	b64 := base64.StdEncoding.EncodeToString(assembled)
	return "gAAAAAC" + b64
}

func resolveUserAgent(userAgent string) string {
	if strings.TrimSpace(userAgent) == "" {
		return defaultUserAgent
	}
	return strings.TrimSpace(userAgent)
}

func randomFloat() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<53))
	f := float64(n.Int64()) / float64(1<<53)
	return strings.TrimRight(fmt.Sprintf("%.17f", f), "0")
}
