#!/usr/bin/env sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <backup-directory>" >&2
  exit 2
fi

backup_dir=$1
test -f "${backup_dir}/SHA256SUMS"
(
  cd "${backup_dir}"
  sha256sum -c SHA256SUMS
  tar -tzf receipts.tar.gz | awk '
    /^\// || /(^|\/)\.\.($|\/)/ { print "Unsafe receipt archive path: " $0 > "/dev/stderr"; exit 1 }
  '
)

echo "Checksums and receipt archive are valid. A database restore test is still required."
