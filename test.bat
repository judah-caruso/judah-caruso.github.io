@echo off

call build.bat &&^
python -m http.server -b 127.0.0.1 -d docs
