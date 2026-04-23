// Package mediasvc implements the ONVIF Media Service over SOAP/HTTP.
//
// The simulator does not run an RTP server or snapshot endpoint itself.
// GetStreamUri and GetSnapshotUri return pass-through URIs that the user
// configured in ProfileConfig — pointing at an external process
// (e.g. ffmpeg, GStreamer) that owns the actual media pipeline.
package mediasvc
