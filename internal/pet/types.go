package pet

type Settings struct {
	Enabled    bool  `json:"enabled"`
	Animate    bool  `json:"animate"`
	Topmost    bool  `json:"topmost"`
	X          int32 `json:"x"`
	Y          int32 `json:"y"`
	Positioned bool  `json:"positioned"`
}
type Diagnostics struct {
	Visible         bool   `json:"visible"`
	TimerActive     bool   `json:"timerActive"`
	Renders         uint64 `json:"renders"`
	NativeWindow    bool   `json:"nativeWindow"`
	Phase           string `json:"phase"`
	Stage           string `json:"stage"`
	Reactions       uint64 `json:"reactions"`
	DPI             uint32 `json:"dpi"`
	FontPixels      int    `json:"fontPixels"`
	ControlsVisible bool   `json:"controlsVisible"`
}
