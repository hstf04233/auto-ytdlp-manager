# auto-ytdlp-manager

![Example of what the web ui looks like](./.github/webui_example.png)

Simple web ui program for archiving videos, and downloading YouTube livestreams in real time!

I really only built this for myself because I needed something that supports yt-dlp and ytarchive at the same time.

This program is in really early alpha, expect bugs...

**I only serve windows binaries currently!**
_(You are able to build from source on other platforms, however I've only tested building on linux. No idea if building/ running on other platforms would work correctly...)_


# Installation

Download autoytdlpmanager-windows.zip from the [latest release](https://github.com/hstf04233/auto-ytdlp-manager/releases/latest)

Extract the contents of autoytdlpmanager-windows.zip into a new folder then run autoytdlpmanager.exe
The program should open and tell you where the server is being hosted, by default it's localhost:8867 (The server port can be changed in config.json)

To create the admin account, go to the localhost server ( by default it's https://localhost:8867 ) and it will give you a page to create the account.
Alternatively, if you wish, you can create the admin account by running ' ./autoytdlpmanager --create-admin-user "username" --create-admin-password "password" '

config.json is created after you run autoytdlpmanager.exe the first time.
Editing config.json while the program is running does not reload the config. Please just edit the config in the web ui config tab or restart the program for changes to apply.

If the program closes after a few seconds (or it closes instantly) then you might have missing dependencies. Read the program log (log_current.log) to confirm.

# Dependencies

To quickly install yt-dlp and ffmpeg, you can run 'winget install yt-dlp.yt-dlp' in a windows command prompt running as administrator. This will not download the ytarchive dependency, you have to get ytarchive below:

This program depends on:
- yt-dlp ([github.com/yt-dlp/yt-dlp](https://github.com/yt-dlp/yt-dlp))
- ytarchive ([This forked version by dreammu specificly](https://github.com/dreammu/ytarchive))
- ffmpeg (and ffprobe) ([ffmpeg.org/download.html](https://ffmpeg.org/download.html), I recommend the builds from [www.gyan.dev/ffmpeg/builds/](https://www.gyan.dev/ffmpeg/builds/))

Although it's supported to just put these dependencies straight into the program's directory,
it's recommended that you add these to your system path environment!
If you need, you can edit the paths yourself in config.json


# Self hosting
For versions v0.20 and up!!! (v0.20 has authentication)

(Not a full entire guide, you must know a little bit about reverse proxies n shit. This will just explain what you have to configure on the program's end.)

To self host this program on a public domain you must:

Run the program locally and create the admin account then exit the program.

Edit 'ServerPort' in config.json to a preferred port, or just leave the default as '8867' if you desire.

Make sure you configure 'IpStrategy' in config.json correctly to prevent IP spoofing, make sure you set this value accordingly to your proxy:
- If using Cloudflare/ Cloudflare tunnel, the 'IpStrategy' value should be: 'cloudflare'
- If using NGINX, the 'IpStrategy' value should be: 'real_ip' or 'forwarded'. Make sure your NGINX server handles the 'X-Real-IP' and 'X-Forwarded-For' headers.
- You can also specify a custom IP address header by setting the 'IpStrategy' value to 'HEADER:Your-Ip-Header-Here'
- If ABSOLUTELY know what you are doing... You can just leave 'IpStrategy' as 'direct' and hope ur shit doesn't get beamed 🥀 (This is not recommended as this means you might be sending raw unencrypted http requests over the air... Please just use a proxy)

If you do everything correctly, you should just be able to run the program and use this wherever you have it hosted!


