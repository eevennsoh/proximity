#!/bin/bash

# Check if at least one query is provided
if [ "$#" -eq 0 ]; then
  echo "Usage: $0 <query1> [query2] ..."
  exit 1
fi

# Temporary file to store concatenated output
TMP_FILE=$(mktemp)

# Loop through all provided queries
for query in "$@"; do
  atlas ml aigateway model list -x "$query" -e stg-east | \
  perl -pe 's/\x1B\[[0-9;?]*[a-zA-Z]//g' | \
  awk -F'│' 'NF>=3 { 
    sn=$2; 
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", sn); 
    mid=$3; 
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", mid); 
    if (sn ~ /^[0-9]+$/ && mid != "") print mid 
  }' >> "$TMP_FILE"
done

# Output the final concatenated result
cat "$TMP_FILE"

# Clean up the temporary file
rm "$TMP_FILE"
