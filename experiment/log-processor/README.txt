This script processes a master log and creates a csv file recording the matches against different regular expressions related to endpoints.

Usage:
inputFilePath and resultFileName are mandatory fields

$ ./process.sh
missing parameters, please provide inputFilePath

$ ./process.sh -i=rohan.txt
missing parameters, please provide resultFileName

To run using defaults:
$ ./process.sh -i=rohan.txt -o=result.csv
>> inputFilePath:  rohan.txt
>> separatorLine:  ----------------
>> resultFileName:  today.csv
>> separator for result file: ,
>> prefixForSubFile:  res
>> skipSplitting:  false

Outcome would be a collection of subfiles and the desired outputfile:

$ ls -l
res1.csv
res2.csv
...
result.csv

If we change the endpoints collection to match against and want to reuse already splitted files (skip the splitting process):
$ ./process.sh -i=rohan.txt -o=result.csv --skipSplitting=true

This is to use another internal file name different than default (res):
./process.sh -i=rohan.txt -o=today_4.csv -p=tem 

This would be to override the default internal name and also avoid processing the splitting again:
./process.sh -i=rohan.txt -o=today_4.csv -p=tem --skipSplitting=true


