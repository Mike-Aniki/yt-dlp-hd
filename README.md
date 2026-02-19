# yt-dlp-hd

A lightweight wrapper around yt-dlp to enforce high resolution (up to 4K) downloading of YouTube videos via Playnite's Extra Metadata Loader add-on — with improved codec compatibility for reliable playback inside Playnite.

Recent YouTube 4K videos are often delivered using AV1 or VP9 codecs, which may not play correctly in Playnite due to Windows Media Foundation limitations.
This fork adds a user-friendly compatibility mode and optional smart H.264 auto-conversion.

## What This fork Version Adds

- Optional H.264-only compatibility mode
- Automatic detection of video codec
- Automatic re-encoding to H.264 only if required
- Configurable quality settings (CRF / preset)
- Backward compatible with the original INI configuration

## Installation

1. Download the latest release and extract the zip file into folder "yt-dlp-hd".
2. Place the folder in a suitable folder such as C:\Playnite\video-tools.
3. Update the included yt-dlp.ini file as per the settings below.
4. In ExtraMetadataLoader's settings, point the YT-DLP path to the wrapper's folder.
5. Restart Playnite (important!)
6. Test download a YouTube video trailer for an existing game. Ensure the video you want to download is high res (not all are).

You can watch a video of the setup process here: https://www.youtube.com/watch?v=zXiarzc5iJA

## Recommended Folder Structure

Example:
```ini
C:\Playnite\video-tools\yt-dlp-hd\
    yt-dlp.exe        ← wrapper (this project)
    yt-dlp.ini

C:\Playnite\video-tools\yt-dlp\
    yt-dlp.exe        ← official yt-dlp

C:\Playnite\video-tools\ffmpeg\bin\
    ffmpeg.exe
    ffprobe.exe
```

## Configuration

Edit the `yt-dlp.ini` file in the wrapper's folder:

### INI Settings

- maxres - Set the maximum resolution for the video. Available options are: Best, 4k, 1080p, 720p, 480p. If the resolution you specifiy isn't available to download then it will grab the next best quality.
- yt-dlp-path - Set the path to the folder where the original yt-dlp.exe binary is stored.
- ffmpeg-path - Set the path to the folder where the ffmpeg.exe binary is installed.
- debug - Set to "true" to log the output to yt-dlp.log or false for no logging.

New: Compatibility Mode

- always_compatible - Set true for downloads H.264 only. May limit downloads to 1080p, since YouTube rarely provides 4K in H.264.
                    - Set false for downloads best available quality (including 4K AV1/VP9). Automatically re-encodes to H.264 only if needed. Preserves 4K resolution. Slightly slower due to conversion step.

### Re-Encode Quality Settings

Only used when ``always_compatible=false`` and the downloaded video is not already H.264.

- x264_preset
Encoding speed/compression balance.
Options: fast, medium, slow.
Recommended: fast.

- x264_crf
Quality level (lower = better quality, larger file size).
Recommended:
1080p → 18
4K → 20

- audio_bitrate
Audio quality (e.g., 192k).


#### Example INI File:

```ini
maxres=4k
yt-dlp-path=C:\Playnite\video-tools\yt-dlp
ffmpeg-path=C:\Playnite\video-tools\ffmpeg\bin
debug=false

always_compatible=false

x264_preset=fast
x264_crf=20
audio_bitrate=192k
```

## How It Works

- The wrapper enforces the desired resolution.
- If always_compatible=true, it forces H.264 streams only.
- If always_compatible=false, it downloads the best quality available.
- After download, the wrapper:
- Detects the video codec using ffprobe.
- If already H.264 → nothing happens.
- If AV1/VP9 → automatically re-encodes to H.264.
- The final output is an MP4 fully compatible with Playnite.

## Why This Fork Exists

YouTube increasingly delivers high-resolution content using AV1 and VP9 codecs.
While efficient, these codecs are not always reliably supported by Windows Media Foundation, which Playnite uses for video playback.

Symptoms may include:

- Black screen
- Video not starting
- Inconsistent playback

This wrapper fork ensures trailers remain playable without requiring:

- Manual conversion
- Codec packs
- System-level tweaks

## Removing/Uninstalling

To remove or uninstall simply delete the yt-dlp-hd folder and repoint YT-DLP in ExtraMetadata's add-on setting to the original .exe.
