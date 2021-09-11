@echo off

if exist docs (
    rmdir /S /Q docs
)

doc html &&^
move /Y out\html docs &&^
rmdir /S /Q out

echo judahcaruso.com > docs\CNAME
