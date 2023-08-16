# Judah Caruso

[![Build](https://github.com/judah-caruso/judah-caruso.github.io/actions/workflows/static-site.yml/badge.svg?branch=main)](https://github.com/judah-caruso/judah-caruso.github.io/actions/workflows/static-site.yml)

The source code for my [personal site](http://judahcaruso.com).

To build it from source, simply run:

```sh
git clone --recurse-submodules https://github.com/judah-caruso/judah-caruso.github.io
cd judah-caruso.github.io/
go run . # generate site
go run . --server=true --port=8080 # start local server at port 8080
```

## Folder structure

- `riv`: site pages (given to site generator)
- `res`: static resources
- `web`: generated website

## Licensing

All source code within this repository (.go, .htm, .css) is licensed under [MIT](./LICENSE). Images, text, and audio files (.png, .riv, .ogg) are licensed under [CC-BY-SA-4.0](./LICENSE.CC-BY-SA-4.0).

