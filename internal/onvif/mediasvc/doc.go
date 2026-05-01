// Package mediasvc implements the ONVIF Media Service over SOAP/HTTP.
//
// The simulator hosts both the RTP stream (via the embedded RTSP server)
// and JPEG snapshots (via the embedded snapshot endpoint) itself when
// ProfileConfig.MediaFilePath is set. GetStreamUri and GetSnapshotUri
// can also return pass-through URIs when ProfileConfig.RTSP or
// ProfileConfig.SnapshotURI override the auto-derived simulator URLs —
// useful for pointing a profile at a real external camera.
package mediasvc
