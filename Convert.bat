@echo off

rem setlocal enableextensions enabledelayedexpansion
rem for /R %%i in (*.xlsx) do (
rem	set str=!str! %%i
rem )
rem xlsx_parser.exe --dlua ./lua --dgo ./gen_config --map_sep "=" --array_sep "," --token_begin "[" --token_end "]" --indent "	" %str%
rem xlsx_parser.exe%str%
rem endlocal

rem xlsx_parser.exe --dlua ./lua --dgo ./gen_config --map_sep "=" --array_sep "," --token_begin "[" --token_end "]" --indent "	" --excels "./tst1.xlsx=[tst1],./TST2.xlsx=[TST2,Tst3]" ./REST.xlsx

xlsx_parser.exe --excels "./tst1.xlsx=[tst1],./TST2.xlsx=[TST2,Tst3]" ./REST.xlsx
pause
