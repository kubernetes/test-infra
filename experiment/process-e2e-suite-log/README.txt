This script processes a master log and creates a csv file recording the matches against different regular expressions related to endpoints.
Master log source can be found here: https://github.com/cncf/apisnoop/issues/17#issuecomment-394866106

Usage:
inputFilePath and resultFileName are mandatory fields

This will fail since its missing inputFilePath
$ ./process-e2e-suite-log.sh
missing parameters, please provide inputFilePath

This will fail since its missing resultFileName
$ ./process-e2e-suite-log.sh -i=master-log.txt
missing parameters, please provide resultFileName

To run using defaults:
$ ./process-e2e-suite-log.sh -i=master-log.txt -o=result.csv
>> inputFilePath:  master-log.txt
>> separatorLine:  ----------------
>> resultFileName:  result.csv
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
$ ./process-e2e-suite-log.sh -i=master-log.txt -o=result.csv --skipSplitting=true

This is to use another internal file name different than default (res):
./process-e2e-suite-log.sh -i=master-log.txt -o=today.csv -p=tem 

This would be to override the default internal name and also avoid processing the splitting again:
./process-e2e-suite-log.sh -i=master-log.txt -o=today.csv -p=tem --skipSplitting=true

