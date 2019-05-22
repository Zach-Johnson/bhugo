# Bhugo

Bhugo is a simple tool written in Go that transforms [Bear](https://bear.app/) notes into [Hugo](https://gohugo.io/)-compatible markdown files. It runs in the background so that whenever you make changes to your Bear notes, you can immediately see the updates on your Hugo server.

Bhugo will monitor a directory of Bear notes based off of a configurable tag prefix. For example, if you prefix all your Bear notes as `#blog`  ( `#blog/finance,  #blog/life`  etc.), configure Bhugo to monitor the `blog` prefix. Bhugo will preserve any custom front matter that you add to your Hugo files.

Bhugo does it’s best to stay out of your way, with only a few requirements for how you write your notes:
- Write your notes in markdown compatibility mode.
- The first line of your note is treated as the title and is used to create the Hugo files and insert the title into the Hugo front matter - a note titled `My Great Post` will generate a file called `my-great-post.md`.
- The second line of your note is expected to be hashtags (and optionally other text), which will correlate to either Hugo categories or tags in the front matter.
- You can insert images into your Bear notes and they will be formatted to match the configurable environment variable designating the image directory in your Hugo blog - so save your images in your Hugo site as you would normally and then insert them directly into your Bear note.
- The `draft` tag has special meaning and will specifically mark the post as a draft in the Hugo front matter, for example `#blog/draft`.

- - - -
## **Warning**
Bhugo will **blow away** the body of an existing file in the `CONTENT_DIR` directory if it already exists. For example, if you title a Bear note `My New Post` and there is an existing file called `my-new-post.md` the body of that file will be truncated and replaced with the content from your Bear note. Any custom front matter in that file, however, will be preserved.

- - - -

## Installation
- [Install Go 1.11+](https://golang.org/dl/)
- `go get github.com/Zach-Johnson/bhugo`

## Configuration
Create a `.bhugo` wherever you like - a good spot is in the root of your Hugo site.  You’ll need to configure this file with several values:

```
# Required - substitute your username and Hugo site directory.
DATABASE="/Users/<username>/Library/Group Containers/9K33E3U3T4.net.shinyfrog.bear/Application Data/database.sqlite"
HUGO_DIR=/Users/<username>/my-awesome-blog

# Optional - defaults listed below
CONTENT_DIR=content/blog
IMAGE_DIR=/img/posts
NOTE_TAG=blog
INTERVAL=1s
CATEGORIES=true
TAGS=false
```

Substitute your `username` in the `DATABASE` variable - this is where Bear stores it’s data. Bhugo is `read-only` on this database but if it makes you feel better, back up that file.

`HUGO_DIR` is the root directory of your Hugo blog.

`CONTENT_DIR` is the output directory relative to the `HUGO_DIR` that Bhugo will save posts to.

`IMAGE_DIR` is the image directory relative to `HUGO_DIR/static`.

`NOTE_TAG` is the tag prefix in Bear that Bhugo will monitor.

`INTERVAL` is how often Bhugo will check for changes to Bear notes.
Valid values given by [time.Duration](https://golang.org/pkg/time/#ParseDuration).

`CATEGORIES` is a boolean value indicating that Bhguo will treat Bear hashtags as Hugo categories in the front matter.

`TAGS` is a boolean value indicating that Bhguo will treat Bear hashtags as Hugo tags in the front matter.

- - - -

**Example set up:**

Configure a .`bhugo` file:

![](../assets/imgs/bhugo-file.png?raw=true)

Start up Bhugo from the same directory where your `.bhugo` file is:

![](../assets/imgs/bhugo-start.png?raw=true)


Make some changes to an existing note, with hashtags prefixed with the `NOTE_TAG` value from the `.bhugo` file:

![](../assets/imgs/bhugo-changes.png?raw=true)


You should see the change reflected in your Hugo content file:

![](../assets/imgs/bhugo-content.png?raw=true)


And a message from Bhugo telling you it made an update:

![](../assets/imgs/bhugo-updated.png?raw=true)


## Contributing
Pull requests, feature requests, bug reports, and general feedback are all more than welcome.
