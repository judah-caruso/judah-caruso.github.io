@echo off

if exist docs (
    rmdir /S /Q docs
)

doc html xml &&^
move /Y out\html docs &&^
move /Y out\atom.xml docs\atom.xml &&^
rmdir /S /Q out

echo judahcaruso.com > docs\CNAME
