#!/bin/bash

set -e

function str_to_array {
  eval "local input=\"\$$1\""
  input="$(echo "$input" | awk '
  {
    split($0, chars, "")
    for (i = 1; i <= length($0); i++) {
      if (i > 1) {
        printf(", ")
      }
      printf("\\\\\\\"%s\\\\\\\"", chars[i])
    }
  }
  ')"
  eval "$1=\"$input\""
}

function update {
  str_to_array APPID
  str_to_array SECRET
  awk "
  /APPID/ {
    print \"var APPID = strings.Join([]string{${APPID}}, \\\"\\\")\"
    next
  }
  /SECRET/ {
    print \"var SECRET = strings.Join([]string{${SECRET}}, \\\"\\\")\"
    next
  }
  {
    print
  }
  " access.go > _access.go

  mv _access.go access.go
}

while test -z "$APPID"; do
  echo -n "Please paste your access key ID: (will not be echoed) "
  read -s APPID
  echo
done
while test -z "$SECRET"; do
  echo -n "Please paste your access key SECRET: (will not be echoed) "
  read -s SECRET
  echo
done
update

if test -n "$BUILD_DOCKER"; then
  docker-compose up
  docker-compose rm --force -v
else
  go build
fi

APPID="someappid"
SECRET="somesecret"
update
