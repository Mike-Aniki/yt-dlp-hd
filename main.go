package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var debugMode = true

func logLine(line string) {
	if !debugMode {
		return
	}
	exPath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	exDir := filepath.Dir(exPath)
	logFile := filepath.Join(exDir, "yt-dlp.log")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	_, _ = f.WriteString(fmt.Sprintf("%s  %s\n", timestamp, line))
}

func readINI() map[string]string {
	config := map[string]string{
		"maxres":            "best",
		"yt-dlp-path":       "",
		"ffmpeg-path":       "",
		"debug":             "true",
		"always_compatible": "false",

		// New options
		"output_codec": "h264", // h264 | h265
		"encoder":      "auto", // auto | cpu | nvenc

		// CPU settings
		"x264_preset": "fast",
		"x264_crf":    "18",
		"x265_preset": "fast",
		"x265_crf":    "22",

		// NVENC settings
		"nvenc_preset": "p5",
		"nvenc_cq":     "19",

		"audio_bitrate": "192k",
	}

	exPath, err := os.Executable()
	if err != nil {
		logLine("Failed to resolve executable path: " + err.Error())
		os.Exit(1)
	}
	exDir := filepath.Dir(exPath)

	data, err := ioutil.ReadFile(filepath.Join(exDir, "yt-dlp.ini"))
	if err != nil {
		logLine("INI not found, using default settings")
		if strings.ToLower(config["debug"]) == "false" {
			debugMode = false
		}
		return config
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			config[strings.ToLower(key)] = val
		}
	}

	if strings.ToLower(config["debug"]) == "false" {
		debugMode = false
	}

	return config
}

func buildBestFormatString(maxres string) string {
	switch strings.ToLower(maxres) {
	case "480p":
		return "bestvideo[height<=480]+bestaudio/best[height<=480]"
	case "720p":
		return "bestvideo[height<=720]+bestaudio/best[height<=720]"
	case "1080p":
		return "bestvideo[height<=1080]+bestaudio/best[height<=1080]"
	case "4k":
		return "bestvideo[height<=2160]+bestaudio/best[height<=2160]"
	default:
		return "bestvideo+bestaudio/best"
	}
}

// Force H.264 (avc1). This often limits YouTube to <=1080p.
func buildH264OnlyFormatString(maxres string) string {
	switch strings.ToLower(maxres) {
	case "480p":
		return "bestvideo[vcodec^=avc1][height<=480]+bestaudio/best[vcodec^=avc1][height<=480]"
	case "720p":
		return "bestvideo[vcodec^=avc1][height<=720]+bestaudio/best[vcodec^=avc1][height<=720]"
	case "1080p":
		return "bestvideo[vcodec^=avc1][height<=1080]+bestaudio/best[vcodec^=avc1][height<=1080]"
	case "4k":
		return "bestvideo[vcodec^=avc1][height<=2160]+bestaudio/best[vcodec^=avc1][height<=2160]"
	default:
		return "bestvideo[vcodec^=avc1]+bestaudio/best[vcodec^=avc1]"
	}
}

func resolveOutputPath(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" {
			out := args[i+1]
			if strings.Contains(out, "%(ext)") {
				out = strings.ReplaceAll(out, "%(ext)s", "mp4")
			}
			if strings.Contains(out, "%(") {
				return ""
			}
			if strings.HasSuffix(out, string(os.PathSeparator)) {
				return ""
			}
			abs, err := filepath.Abs(out)
			if err == nil {
				return abs
			}
			return out
		}
	}
	return ""
}

func ffprobeCodec(ffprobeExe, filePath string) (string, error) {
	cmd := exec.Command(
		ffprobeExe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=nw=1:nk=1",
		filePath,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe failed: %v (%s)", err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func ffmpegHasEncoder(ffmpegExe, encoderName string) bool {
	// Check if ffmpeg lists an encoder (e.g., h264_nvenc, hevc_nvenc)
	cmd := exec.Command(ffmpegExe, "-hide_banner", "-encoders")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		logLine("ffmpeg -encoders failed: " + err.Error())
		return false
	}
	return strings.Contains(out.String(), encoderName)
}

func normalizeOutputCodec(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "h265", "hevc":
		return "h265"
	default:
		return "h264"
	}
}

func normalizeEncoderMode(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "cpu":
		return "cpu"
	case "nvenc":
		return "nvenc"
	default:
		return "auto"
	}
}

func targetCodecName(outputCodec string) string {
	// Compare against ffprobe codec_name values
	if outputCodec == "h265" {
		return "hevc"
	}
	return "h264"
}

func buildFfmpegReencodeArgs(config map[string]string, outputCodec, encoderMode string, ffmpegExe string) (string, []string) {
	// Returns: chosen encoder label for logging + ffmpeg arguments (excluding -y -i input and output file)
	audioBitrate := config["audio_bitrate"]

	outputCodec = normalizeOutputCodec(outputCodec)
	encoderMode = normalizeEncoderMode(encoderMode)

	// Decide actual encoder (auto => nvenc if available else cpu)
	useNvenc := false
	if encoderMode == "nvenc" {
		useNvenc = true
	} else if encoderMode == "auto" {
		if outputCodec == "h264" && ffmpegHasEncoder(ffmpegExe, "h264_nvenc") {
			useNvenc = true
		}
		if outputCodec == "h265" && ffmpegHasEncoder(ffmpegExe, "hevc_nvenc") {
			useNvenc = true
		}
	}

	if useNvenc {
		nvPreset := config["nvenc_preset"]
		nvCQ := config["nvenc_cq"]

		if outputCodec == "h265" {
			return "hevc_nvenc", []string{
				"-c:v", "hevc_nvenc",
				"-preset", nvPreset,
				"-cq", nvCQ,
				"-c:a", "aac",
				"-b:a", audioBitrate,
				"-movflags", "+faststart",
			}
		}

		return "h264_nvenc", []string{
			"-c:v", "h264_nvenc",
			"-preset", nvPreset,
			"-cq", nvCQ,
			"-c:a", "aac",
			"-b:a", audioBitrate,
			"-movflags", "+faststart",
		}
	}

	// CPU path
	if outputCodec == "h265" {
		preset := config["x265_preset"]
		crf := config["x265_crf"]
		return "libx265", []string{
			"-c:v", "libx265",
			"-preset", preset,
			"-crf", crf,
			"-c:a", "aac",
			"-b:a", audioBitrate,
			"-movflags", "+faststart",
		}
	}

	preset := config["x264_preset"]
	crf := config["x264_crf"]
	return "libx264", []string{
		"-c:v", "libx264",
		"-preset", preset,
		"-crf", crf,
		"-c:a", "aac",
		"-b:a", audioBitrate,
		"-movflags", "+faststart",
	}
}

func reencode(ffmpegExe, inPath, encoderLabel string, ffmpegArgs []string) error {
	tmpPath := inPath + ".tmp.mp4"

	args := []string{"-y", "-i", inPath}
	args = append(args, ffmpegArgs...)
	args = append(args, tmpPath)

	logLine("FFmpeg encoder: " + encoderLabel)
	logLine("FFmpeg args: " + strings.Join(args, " "))

	cmd := exec.Command(ffmpegExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	if err := os.Remove(inPath); err != nil {
		return err
	}
	return os.Rename(tmpPath, inPath)
}

func main() {
	args := os.Args[1:]
	config := readINI()

	maxres := config["maxres"]
	alwaysCompatible := strings.ToLower(config["always_compatible"]) == "true"
	outputCodec := normalizeOutputCodec(config["output_codec"])
	encoderMode := normalizeEncoderMode(config["encoder"])

	ytDlpPath := filepath.Join(config["yt-dlp-path"], "yt-dlp.exe")
	ffmpegDir := config["ffmpeg-path"]

	logLine("Original args: " + strings.Join(args, " "))
	logLine(fmt.Sprintf("Config: maxres=%s always_compatible=%v output_codec=%s encoder=%s",
		maxres, alwaysCompatible, outputCodec, encoderMode))

	if outputCodec == "h265" {
		logLine("Note: H.265/HEVC playback may require Windows HEVC support. H.264 is recommended for maximum compatibility.")
	}

	// always_compatible means "download H.264 only" (best Playnite compatibility)
	var format string
	if alwaysCompatible {
		format = buildH264OnlyFormatString(maxres)
		if outputCodec != "h264" {
			logLine("always_compatible=true forces H.264 download for compatibility (ignoring output_codec=h265).")
		}
	} else {
		format = buildBestFormatString(maxres)
	}

	var finalArgs []string
	skipNext := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if skipNext {
			skipNext = false
			continue
		}

		if arg == "-f" && i+1 < len(args) && args[i+1] == "mp4" {
			logLine("Stripped -f mp4")
			skipNext = true
			continue
		}

		finalArgs = append(finalArgs, arg)
	}

	finalArgs = append(finalArgs, "-f", format)

	if ffmpegDir != "" {
		finalArgs = append(finalArgs, "--ffmpeg-location", ffmpegDir)
		logLine("Set ffmpeg path: " + ffmpegDir)
	}
	finalArgs = append(finalArgs, "--merge-output-format", "mp4")
	finalArgs = append(finalArgs, "--no-keep-video")

	// Fix static -o output (e.g., VideoTemp -> VideoTemp.%(ext)s)
	for i := 0; i < len(finalArgs)-1; i++ {
		if finalArgs[i] == "-o" {
			out := finalArgs[i+1]
			if !strings.Contains(out, "%(") {
				if strings.HasSuffix(strings.ToLower(out), ".mp4") {
					out = strings.TrimSuffix(out, ".mp4")
					logLine("Stripped .mp4 extension from output path")
				}
				out += ".%(ext)s"
				finalArgs[i+1] = out
				logLine("Adjusted output to: " + out)
			}
		}
	}

	logLine("Final yt-dlp args: " + strings.Join(finalArgs, " "))

	cmd := exec.Command(ytDlpPath, finalArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir, _ = filepath.Abs(".")
	if err := cmd.Run(); err != nil {
		logLine("Error running yt-dlp: " + err.Error())
		os.Exit(1)
	}

	// Smart re-encode only when needed (only when always_compatible=false)
	if alwaysCompatible || ffmpegDir == "" {
		return
	}

	outPath := resolveOutputPath(finalArgs)
	if outPath == "" {
		logLine("Could not resolve output path from -o; skipping codec check/re-encode.")
		return
	}

	if _, statErr := os.Stat(outPath); statErr != nil {
		logLine("Output file not found for codec check: " + outPath + " (" + statErr.Error() + ")")
		return
	}

	ffprobeExe := filepath.Join(ffmpegDir, "ffprobe.exe")
	ffmpegExe := filepath.Join(ffmpegDir, "ffmpeg.exe")

	codec, probeErr := ffprobeCodec(ffprobeExe, outPath)
	if probeErr != nil {
		logLine("ffprobe error: " + probeErr.Error())
		return
	}
	logLine("Detected video codec: " + codec)

	target := targetCodecName(outputCodec)
	if strings.ToLower(codec) == target {
		logLine("Already " + strings.ToUpper(target) + ", skipping re-encode.")
		return
	}

	encoderLabel, ffArgs := buildFfmpegReencodeArgs(config, outputCodec, encoderMode, ffmpegExe)
	logLine(fmt.Sprintf("Re-encoding to %s using %s...", strings.ToUpper(target), encoderLabel))

	if encErr := reencode(ffmpegExe, outPath, encoderLabel, ffArgs); encErr != nil {
		logLine("Re-encode error: " + encErr.Error())
		os.Exit(1)
	}
	logLine("Re-encode done.")
}