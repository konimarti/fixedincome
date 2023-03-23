#!/bin/zsh
 curl -X GET https://data.snb.ch/api/cube/rendopar/data/csv/en | tail -6 | awk -F'"' '{print $4"="$6}' | jo | jq '. += {"spread": 0.0}'
