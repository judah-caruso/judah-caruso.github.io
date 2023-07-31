# Judah Caruso

[![Build](https://github.com/judah-caruso/judah-caruso.github.io/actions/workflows/static-site.yml/badge.svg?branch=main)](https://github.com/judah-caruso/judah-caruso.github.io/actions/workflows/static-site.yml)

The source code for my [personal site](http://judahcaruso.com).

To build it from source, simply run:

```
git clone --recurse-submodules https://github.com/judah-caruso/judah-caruso.github.io
cd judah-caruso.github.io/
zig build run
```

## Folder structure

- `src`: site generator code
- `riv`: site pages (given to site generator)
- `res`: static resources
- `web`: generated website

## Licensing

All source code within this repository (.zig, .htm, .css) is licensed under [MIT](./LICENSE). Images, text, and audio files (.png, .riv, .ogg) are licensed under [CC-BY-SA-4.0](./LICENSE.CC-BY-SA-4.0).

