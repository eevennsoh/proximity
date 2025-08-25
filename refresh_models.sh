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
  atlas ml aigateway model list -x "$query" \
    | sed 's/\x1b\[[0-9;]*m//g' \
    | awk -F '│' '{gsub(/ /,"",$2); if (length($2) > 0 && $2 != "ModelID") print $2}' \
    >> "$TMP_FILE"
done

# Output the final concatenated result
cat "$TMP_FILE"

# Clean up the temporary file
rm "$TMP_FILE"
