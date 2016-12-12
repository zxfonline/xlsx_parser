@echo off
setlocal enableextensions enabledelayedexpansion
 for /R %%i in (*.xlsx) do (
	set str=!str! %%i
)
xlsx_parser.exe --dlua ./ --dgo ./ --map_sep "=" --array_sep "," --token_begin "[" --token_end "]" --indent "\t" %str%
endlocal
pause