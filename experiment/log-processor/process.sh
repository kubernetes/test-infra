#!/bin/bash

# This script processes an "input file" and generates a "output file" formatted in csv.
#
# "input file":
# -------------
# it is a "master log" that is the collection of the endpoints visited while executing a testing run.  
# This file has a known format: 
# a) line 1:  "separatorLine"
# b) line 2: timestamp
# c) line 3: sig-description for the testcase
# d) line 4: description for the testcase
# e) line 5 : path of the filename where this specific testcase is located
# f) line (repeteable...N): endpoint visited, expressed using non-rest-verbs (get, list, create, update, delete, deletecollection, patch, watch)
# g) line N :  "separatorLine"
inputFilePath=
separatorLine=----------------

# "output file":
# -------------
# "resultFileName": the name for the outputFile
# "separatorCharacter": the character used to separate the values 
resultFileName=
separatorCharacter=","

# "endpoints":
# ------------
# "endpoints" is map to keep the association between 
# key: formal-endpoint (HTTP verbs and real url) 
# value: logged-endpoint regular expression (non-rest-verbs and other url expressed) as found in "master log"
# there is also a column to collect if the tescase is conformance or not
declare -A endpoints=(
["Conformance"]="\[Conformance\]"

["POST /api/v1/namespaces/{namespace}/pods"]="create \/api\/v1\/namespaces\/.*\/pods$"
["POST /api/v1/namespaces/{namespace}/pods/{name}/eviction"]="create \/api\/v1\/namespaces\/.*\/pods\/.*\/eviction$"
["PATCH /api/v1/namespaces/{namespace}/pods/{name}"]="patch \/api\/v1\/namespaces\/.*\/pods\/[^\/]*$"
["PUT /api/v1/namespaces/{namespace}/pods/{name}"]="update \/api\/v1\/namespaces\/.*\/pods\/[^\/]*$"
["DELETE /api/v1/namespaces/{namespace}/pods/{name}"]="delete \/api\/v1\/namespaces\/.*\/pods\/[^\/]*$"
["DELETE /api/v1/namespaces/{namespace}/pods"]="deletecollection \/api\/v1\/namespaces\/.*\/pods$"

["GET /api/v1/namespaces/{namespace}/pods/{name}"]="get \/api\/v1\/namespaces\/.*\/pods\/[^\/]*$"
["GET /api/v1/namespaces/{namespace}/pods"]="list \/api\/v1\/namespaces\/.*\/pods$"
["GET /api/v1/pods"]="list \/api\/v1\/pods$"
["GET /api/v1/watch/namespaces/{namespace}/pods/{name}"]="watch \/api\/v1\/namespaces\/.*\/pods\/[^\/]*$"
["GET /api/v1/watch/namespaces/{namespace}/pods"]="watch \/api\/v1\/namespaces\/.*\/pods$"
["GET /api/v1/watch/pods"]="watch \/api\/v1\/pods$"

["PATCH /api/v1/namespaces/{namespace}/pods/{name}/status"]="patch \/api\/v1\/namespaces\/.*\/pods\/.*\/status$"
["GET /api/v1/namespaces/{namespace}/pods/{name}/status"]="get \/api\/v1\/namespaces\/.*\/pods\/.*\/status$"
["PUT /api/v1/namespaces/{namespace}/pods/{name}/status"]="update \/api\/v1\/namespaces\/.*\/pods\/.*\/status$"

["POST /api/v1/namespaces/{namespace}/pods/{name}/portforward"]="create \/api\/v1\/namespaces\/.*\/pods\/.*\/portforward$"
["POST /api/v1/namespaces/{namespace}/pods/{name}/proxy"]="create \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy$"
["POST /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}"]="create \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy\/[^\/]*$"
["DELETE /api/v1/namespaces/{namespace}/pods/{name}/proxy"]="delete \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy$"
["DELETE /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}"]="delete \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy\/[^\/]*$"
["GET /api/v1/namespaces/{namespace}/pods/{name}/portforward"]="list \/api\/v1\/namespaces\/.*\/pods\/.*\/portforward$"
["GET /api/v1/namespaces/{namespace}/pods/{name}/proxy"]="list \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy$"
["GET /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}"]="get \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy\/[^\/]*$"

["PUT /api/v1/namespaces/{namespace}/pods/{name}/proxy"]="update \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy$"
["PUT /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}"]="update \/api\/v1\/namespaces\/.*\/pods\/.*\/proxy\/[^\/]*$"

["GET /api/v1/namespaces/{namespace}/pods/{name}/log"]="get \/api\/v1\/namespaces\/.*\/pods\/.*\/log$"

)

# process steps:
# -------------
# a) "input file" will be split into subFiles. 
#   a.1) The prefix for these subFiles names is in "prefixForSubFile"
# b) header creation
# c) each subFile, will be tested against "endpoints" regular expression collection. Results will be collected in a row per file
prefixForSubFile=res
skipSplitting=false
#------------------------------------------------------------------------------
# mandatory parameters:
# a) inputFilePath 
# b) resultFileName
# optional parameters
# a) skipSplitting : to skip the operation of splitting the main master-log file
# b) prefixForSubFile : to override default name (res) 

for i in "$@"
do
case $i in
    -i=*|--input=*)
    inputFilePath="${i#*=}"
    shift
    ;;
    -o=*|--output=*)
    resultFileName="${i#*=}"
    shift 
    ;;
    --skipSplitting=true)
    skipSplitting=true
    shift 
    ;;
    -p=*|--prefixForSubFile=*)
    prefixForSubFile="${i#*=}"
    shift 
    ;;
esac
done

if [ -z "$inputFilePath" ]
  then
      echo "missing parameters, please provide inputFilePath"	
      exit
fi

if [ -z "$resultFileName" ]
  then
      echo "missing parameters, please provide resultFileName"   
      exit
fi

lookupFileNameExpression="$prefixForSubFile".*
#------------------------------------------------------------------------------
echo ">> inputFilePath: " "$inputFilePath"
echo ">> separatorLine: " "$separatorLine"
echo ">> resultFileName: " "$resultFileName"
echo ">> separator for result file:" "$separatorCharacter"
echo ">> prefixForSubFile: " "$prefixForSubFile"
echo ">> skipSplitting: " "$skipSplitting"

#------------------------------------------------------------------------------
#a) splitting the main-file into test-files 
if [ "$skipSplitting" == false ]
  then
    csplit --quiet --prefix="$prefixForSubFile". "$inputFilePath" /"$separatorLine"/ {*}
fi

#------------------------------------------------------------------------------
# b) header creation
header="filename $separatorCharacter testDescription"

for endpoint in "${!endpoints[@]}"
do
  header="$header $separatorCharacter $endpoint" 
done

echo "$header" >> "$resultFileName"

#------------------------------------------------------------------------------
# c) processing each subFile
for currentFile in `find . -maxdepth 1 -type f -name "$lookupFileNameExpression" `
do
    testName=`sed -n '3,4 p' "$currentFile"| tr -d '\n' | tr "$separatorCharacter" ' '`

    line="$currentFile $separatorCharacter $testName"

    for endpoint in "${!endpoints[@]}"
    do
      exp="${endpoints[$endpoint]}" 
      
      match=`sed -n "s~$exp~\0~p" "$currentFile"`
      if [ -n "$match" ]
        then
          line="$line $separatorCharacter""Y"
        else
	  line="$line $separatorCharacter"
      fi
    done

    echo "$line" >> "$resultFileName"
done
