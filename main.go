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
		// can't call logLine safely here (it depends on debugMode + file path)
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
	// Backward compatible defaults (original INI keys still work)
	config := map[string]string{
		"maxres":            "best",
		"yt-dlp-path":       "",
		"ffmpeg-path":       "",
		"debug":             "true",  // default to true if not provided
		"always_compatible": "false", // user-friendly: false = best quality + auto-convert if needed

		// Only used when always_compatible=false AND codec isn't already H.264
		"x264_preset":   "fast",
		"x264_crf":      "18",
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
		// Apply debug switch from defaults
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

// Best quality (may pick AV1/VP9 in 4K)
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
	// Try to resolve -o output template to an actual mp4 path.
	// Works best with simple templates, especially the wrapper's own "VideoTemp.%(ext)s" style.
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" {
			out := args[i+1]

			// If it uses %(ext)s, replace with mp4
			if strings.Contains(out, "%(ext)") {
				out = strings.ReplaceAll(out, "%(ext)s", "mp4")
			}

			// If it still contains other tokens, we can't reliably resolve
			if strings.Contains(out, "%(") {
				return ""
			}

			// If it's a directory-only output, can't resolve
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
	return strings.TrimSpace(out.String()), nil // "h264", "av1", "vp9", ...
}

func reencodeToH264(ffmpegExe, inPath, preset, crf, audioBitrate string) error {
	tmpPath := inPath + ".tmp.mp4"

	args := []string{
		"-y",
		"-i", inPath,
		"-c:v", "libx264",
		"-preset", preset,
		"-crf", crf,
		"-c:a", "aac",
		"-b:a", audioBitrate,
		"-movflags", "+faststart",
		tmpPath,
	}

	cmd := exec.Command(ffmpegExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	// Replace original file with the converted one
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

	ytDlpPath := filepath.Join(config["yt-dlp-path"], "yt-dlp.exe")
	ffmpegDir := config["ffmpeg-path"] // expected to be the folder containing ffmpeg.exe (+ ffprobe.exe)

	logLine("Original args: " + strings.Join(args, " "))
	logLine(fmt.Sprintf("Config: maxres=%s always_compatible=%v", maxres, alwaysCompatible))

	var format string
	if alwaysCompatible {
		format = buildH264OnlyFormatString(maxres)
	} else {
		format = buildBestFormatString(maxres)
	}

	var finalArgs []string
	skipNext := false

	// Clean up incoming args from caller (Playnite/EML)
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if skipNext {
			skipNext = false
			continue
		}

		// Remove "-f mp4" if the caller forces it (we provide our own -f)
		if arg == "-f" && i+1 < len(args) && args[i+1] == "mp4" {
			logLine("Stripped -f mp4")
			skipNext = true
			continue
		}

		finalArgs = append(finalArgs, arg)
	}

	// Add corrected format
	finalArgs = append(finalArgs, "-f", format)

	// Add ffmpeg location and merge format (still produce mp4)
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

			// If it does not contain a template token...
			if !strings.Contains(out, "%(") {
				// If it ends in .mp4, strip it (we'll reattach via template)
				if strings.HasSuffix(strings.ToLower(out), ".mp4") {
					out = strings.TrimSuffix(out, ".mp4")
					logLine("Stripped .mp4 extension from output path")
				}
				// Append .%(ext)s template
				out += ".%(ext)s"
				finalArgs[i+1] = out
				logLine("Adjusted output to: " + out)
			}
		}
	}

	logLine("Final yt-dlp args: " + strings.Join(finalArgs, " "))

	// Run yt-dlp (real binary)
	cmd := exec.Command(ytDlpPath, finalArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir, _ = filepath.Abs(".")
	err := cmd.Run()
	if err != nil {
		logLine("Error running yt-dlp: " + err.Error())
		os.Exit(1)
	}

	// If user wants best quality, auto-convert ONLY when needed
	if !alwaysCompatible && ffmpegDir != "" {
		outPath := resolveOutputPath(finalArgs)
		if outPath == "" {
			logLine("Could not resolve output path from -o; skipping codec check/re-encode.")
			return
		}

		// Only proceed if file exists
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

		if strings.ToLower(codec) == "h264" {
			logLine("Already H.264, skipping re-encode.")
			return
		}

		preset := config["x264_preset"]
		crf := config["x264_crf"]
		abr := config["audio_bitrate"]

		logLine(fmt.Sprintf("Re-encoding to H.264 (preset=%s crf=%s audio=%s)...", preset, crf, abr))
		if encErr := reencodeToH264(ffmpegExe, outPath, preset, crf, abr); encErr != nil {
			logLine("Re-encode error: " + encErr.Error())
			os.Exit(1)
		}
		logLine("Re-encode done.")
	}
}
