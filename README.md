# auto-ytdlp-manager

Very simple web ui program written in go that is a wrapper for yt-dlp and ytarchive!

I really only built this for myself because I needed something that supports yt-dlp and ytarchive at the same time.

**This is currently a windows only program!!**
_(You might be able to build from source on other platforms, although I haven't tested if this program works on platforms other than windows. (swoon))_


# Installation

Create a new folder and put "autoytdlpmanager.exe" in it.
Then run autoytdlpmanager.exe
A command prompt should open and tell you where the server is being hosted, by default it's localhost:8867 (The port can be changed in config.json)

config.json is created after you run autoytdlpmanager.exe the first time.
Editing config.json while the program is running does not reload the config. Please just edit the config in the web ui config tab or restart the program for changes to apply.

If the program closes after a few seconds (or it closes instantly) then you might have missing dependencies. Read the program log (log_current.log) to confirm.

# Dependencies

This program depends on:
- yt-dlp
- ytarchive ([This forked version by dreammu specificly](https://github.com/dreammu/ytarchive))
- ffmpeg

Although it's supported to just put these dependencies straight into the program's directory,
it's recommended that you add these to your system path environment!
If you need, you can edit the paths yourself in config.json

