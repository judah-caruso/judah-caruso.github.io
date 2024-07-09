#!/usr/bin/env sh

go run . &&\
python3 -m http.server -d web
