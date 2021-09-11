@echo off

doc html &&^
python -m http.server -b 127.0.0.1 -d out\html
