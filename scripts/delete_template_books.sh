#!/bin/bash
# file: scripts/delete_template_books.sh
# version: 1.0.0
# guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

# Delete the bad database and force a clean rescan
echo "Deleting database with template path records..."
rm -rf audiobooks.pebble
echo "Database deleted. Restart the server and click 'Full Rescan' to rebuild from actual files."
