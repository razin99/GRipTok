# GRipTok
- Download all videos from a user on TikTok
- Not very efficient yet, still does about 1 video every second
- And may occasionally (or frequently) partially download files

## Download strategy
- Get first video
- Download video in a go routine
- Press down arrow, and repeat until pressing down arrow does not navigate page

# Future plans
- Get all video links by 'scrolling' all the way down
- Then use a pool of workers to navigate and download
