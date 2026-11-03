// Wails runtime 的浏览器端替身（仅用于宣传片素材采集），由 vite alias 注入。
const ok = (v?: any) => Promise.resolve(v)
export function LogPrint(..._a: any[]) { return ok() }
export function LogTrace(..._a: any[]) { return ok() }
export function LogDebug(..._a: any[]) { return ok() }
export function LogInfo(..._a: any[]) { return ok() }
export function LogWarning(..._a: any[]) { return ok() }
export function LogError(..._a: any[]) { return ok() }
export function LogFatal(..._a: any[]) { return ok() }
export function EventsOnMultiple(..._a: any[]) { return () => {} }
export function EventsOn(..._a: any[]) { return () => {} }
export function EventsOff(..._a: any[]) { return ok() }
export function EventsOffAll(..._a: any[]) { return ok() }
export function EventsOnce(..._a: any[]) { return () => {} }
export function EventsEmit(..._a: any[]) { return ok() }
export function WindowReload(..._a: any[]) { return ok() }
export function WindowReloadApp(..._a: any[]) { return ok() }
export function WindowSetAlwaysOnTop(..._a: any[]) { return ok() }
export function WindowSetSystemDefaultTheme(..._a: any[]) { return ok() }
export function WindowSetLightTheme(..._a: any[]) { return ok() }
export function WindowSetDarkTheme(..._a: any[]) { return ok() }
export function WindowCenter(..._a: any[]) { return ok() }
export function WindowSetTitle(..._a: any[]) { return ok() }
export function WindowFullscreen(..._a: any[]) { return ok() }
export function WindowUnfullscreen(..._a: any[]) { return ok() }
export function WindowIsFullscreen() { return ok(false) }
export function WindowGetSize() { return ok({ w: 680, h: 680 }) }
export function WindowSetSize(..._a: any[]) { return ok() }
export function WindowSetMaxSize(..._a: any[]) { return ok() }
export function WindowSetMinSize(..._a: any[]) { return ok() }
export function WindowSetPosition(..._a: any[]) { return ok() }
export function WindowGetPosition() { return ok({ x: 0, y: 0 }) }
export function WindowHide(..._a: any[]) { return ok() }
export function WindowShow(..._a: any[]) { return ok() }
export function WindowMaximise(..._a: any[]) { return ok() }
export function WindowToggleMaximise(..._a: any[]) { return ok() }
export function WindowUnmaximise(..._a: any[]) { return ok() }
export function WindowIsMaximised() { return ok(false) }
export function WindowMinimise(..._a: any[]) { return ok() }
export function WindowUnminimise(..._a: any[]) { return ok() }
export function WindowSetBackgroundColour(..._a: any[]) { return ok() }
export function ScreenGetAll() { return ok([]) }
export function WindowIsMinimised() { return ok(false) }
export function WindowIsNormal() { return ok(false) }
export function BrowserOpenURL(..._a: any[]) { return ok() }
export function Environment() { return ok({ buildType: 'production', platform: 'darwin', arch: 'arm64' }) }
export function Quit(..._a: any[]) { return ok() }
export function Hide(..._a: any[]) { return ok() }
export function Show(..._a: any[]) { return ok() }
export function ClipboardGetText(..._a: any[]) { return ok() }
export function ClipboardSetText(..._a: any[]) { return ok() }
export function OnFileDrop(..._a: any[]) { return ok() }
export function OnFileDropOff(..._a: any[]) { return ok() }
export function CanResolveFilePaths() { return ok(false) }
export function ResolveFilePaths(..._a: any[]) { return ok() }
export function InitializeNotifications(..._a: any[]) { return ok() }
export function CleanupNotifications(..._a: any[]) { return ok() }
export function IsNotificationAvailable() { return ok(false) }
export function RequestNotificationAuthorization(..._a: any[]) { return ok() }
export function CheckNotificationAuthorization() { return ok(false) }
export function SendNotification(..._a: any[]) { return ok() }
export function SendNotificationWithActions(..._a: any[]) { return ok() }
export function RegisterNotificationCategory(..._a: any[]) { return ok() }
export function RemoveNotificationCategory(..._a: any[]) { return ok() }
export function RemoveAllPendingNotifications(..._a: any[]) { return ok() }
export function RemovePendingNotification(..._a: any[]) { return ok() }
export function RemoveAllDeliveredNotifications(..._a: any[]) { return ok() }
export function RemoveDeliveredNotification(..._a: any[]) { return ok() }
export function RemoveNotification(..._a: any[]) { return ok() }
