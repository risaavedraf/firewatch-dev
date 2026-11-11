#!/usr/bin/env sh
# Pins the delivery overlay's images from an image list.
#
# The base renders `firewatch/<service>:replace-before-deploy` on purpose: it is
# a template, and a digest written into it would name one build for every
# environment. This script writes the registry host and the digest into
# overlays/delivery/kustomization.yaml, where a digest belongs, so pinning a
# release is a mechanical change instead of a hand-edited tag.
#
# It rewrites those two fields in place and nothing else. The second run with
# the same list leaves the file byte for byte identical. No network, no
# `docker`, no `yq`: `awk` reads the list and the overlay, and the new file is
# installed with `mv` only after every line of the list was accepted, so a
# rejected list leaves the overlay untouched.
#
# Usage: ./pin-images.sh IMAGE_LIST
#        ./pin-images.sh --help
set -eu

here=$(cd "$(dirname "$0")" && pwd)
overlay=$here/kustomization.yaml
base_dir=$here/../../base
prog=pin-images

usage() {
	cat <<'EOF'
Usage: ./pin-images.sh IMAGE_LIST

Pin the delivery overlay's images from IMAGE_LIST, in place.

IMAGE_LIST holds one image per line, in this format:

    <service> <registry>/firewatch/<service>@sha256:<64 lowercase hex>

For example (the digest is abbreviated here; the script wants all 64 digits):

    gateway   registry.example.com/firewatch/gateway@sha256:01d650...0f262a

Every service the base declares needs exactly one line. Blank lines and lines
whose first character is `#` are ignored. A malformed line, a second line for
the same service, a service the base does not declare, and a base service the
list omits are all errors: the message names the line or the service it is
about, and the overlay is left as it was.

The registry host is the one value the tree cannot derive; the digests are
content digests and travel with the image they belong to.
EOF
}

case "${1:-}" in
--help | -h)
	usage
	exit 0
	;;
esac

if [ "$#" -ne 1 ]; then
	usage >&2
	exit 1
fi

list=$1
[ -f "$list" ] || {
	echo "$prog: $list is not a file" >&2
	exit 1
}
[ -f "$overlay" ] || {
	echo "$prog: missing $overlay: that is the file this script pins" >&2
	exit 1
}
[ -d "$base_dir" ] || {
	echo "$prog: missing $base_dir: it declares the services the overlay pins" >&2
	exit 1
}

# The services are read from the base rather than repeated here: a service added
# to the base and never pinned is the defect this list is checked against, and a
# hand-kept copy of the names would not see it. Every application image in the
# base carries the placeholder tag, so the tag is what identifies the set.
services=$(grep -ho 'image: firewatch/[a-z][a-z-]*:replace-before-deploy' \
	"$base_dir"/*.yaml |
	sed 's|^image: firewatch/||; s|:replace-before-deploy$||' | sort -u |
	tr '\n' ' ')
if [ -z "$services" ]; then
	echo "$prog: no firewatch/<service>:replace-before-deploy image under $base_dir: nothing to pin" >&2
	exit 1
fi

tmp=$overlay.tmp.$$
trap 'rm -f "$tmp"' EXIT

# One pass, two inputs: the list (matched by name) and then the overlay. The
# rewrite keeps the file's shape — one `- name:` entry per service, `newName`
# and `digest` under it — and emits the digest as a folded scalar, the form the
# tracked file uses to stay inside 80 columns. The list is matched by file name
# and not by NR == FNR because an empty list makes that test true for the
# overlay's first line, which would read a manifest as a pinned image.
if ! awk -v prog="$prog" -v list="$list" -v overlay="$overlay" \
	-v services="$services" '
	BEGIN {
		n = split(services, names, " ")
		for (i = 1; i <= n; i++) {
			declared[names[i]] = 1
			declared_text = declared_text (i > 1 ? " " : "") names[i]
		}
	}

	function fail(msg) {
		printf "%s: %s: %s\n", prog, where, msg > "/dev/stderr"
		bad = 1
	}

	FILENAME == list {
		line = $0
		sub(/\r$/, "", line)
		sub(/^[ \t]+/, "", line)
		sub(/[ \t]+$/, "", line)
		if (line == "" || substr(line, 1, 1) == "#") next
		where = list ":" FNR

		f = split(line, field, /[ \t]+/)
		if (f != 2) {
			fail("expected \"<service> <image>\", got \"" line "\"")
			next
		}
		svc = field[1]
		img = field[2]
		if (!(svc in declared)) {
			fail("\"" svc "\" is not a service the base declares: it declares " declared_text)
			next
		}
		if (svc in image_of) {
			fail("a second line for " svc ", which line " line_of[svc] " already pinned")
			next
		}
		at = index(img, "@")
		if (at == 0) {
			fail("\"" img "\" carries no @sha256: digest; a tag is what this list replaces")
			next
		}
		newname = substr(img, 1, at - 1)
		digest = substr(img, at + 1)
		if (digest !~ /^sha256:[0-9a-f]+$/) {
			fail("\"" digest "\" is not a sha256: digest in lowercase hex")
			next
		}
		if (length(digest) != 71) {
			fail("\"" digest "\" has " length(digest) - 7 " hex digits, want 64")
			next
		}
		if (newname !~ ("^[^/]+/firewatch/" svc "$")) {
			fail("\"" newname "\" does not name firewatch/" svc " under a registry host")
			next
		}
		image_of[svc] = newname
		digest_of[svc] = digest
		line_of[svc] = FNR
		next
	}

	{
		if ($0 ~ /^images:[ ]*$/) {
			in_images = 1
			print
			next
		}
		if (in_images && $0 ~ /^[^ ]/) {
			in_images = 0
			cur = ""
		}
		if (!in_images) {
			print
			next
		}
		if ($0 ~ /^  - name: /) {
			# The entry is matched to the list by the base image name, so the
			# `firewatch/` repository prefix comes off here: the list speaks of
			# services, and the base is what names them `firewatch/<service>`.
			cur = $0
			sub(/^  - name: /, "", cur)
			sub(/^firewatch\//, "", cur)
			folded = 0
			print
			next
		}
		if (cur in image_of && $0 ~ /^    newName:/) {
			print "    newName: " image_of[cur]
			name_lines[cur]++
			folded = 0
			next
		}
		if (cur in digest_of && $0 ~ /^    digest:/) {
			print "    digest: >-"
			print "      " digest_of[cur]
			digest_lines[cur]++
			folded = 1
			next
		}
		# The value line under a `digest: >-` this run replaced.
		if (folded && $0 ~ /^      /) next
		folded = 0
		print
	}

	END {
		if (bad) exit 1
		missing = ""
		for (s in declared)
			if (!(s in image_of)) missing = missing " " s
		if (missing != "") {
			printf "%s: %s: the base declares these services and the list has no entry for them:%s\n", prog, list, missing > "/dev/stderr"
			exit 1
		}
		half = ""
		for (s in image_of)
			if (name_lines[s] != 1 || digest_lines[s] != 1)
				half = half " " s "(" name_lines[s] + 0 "/" digest_lines[s] + 0 ")"
		if (half != "") {
			printf "%s: %s: one images entry per service needs both newName and digest; newName/digest counts:%s\n", prog, overlay, half > "/dev/stderr"
			exit 1
		}
	}
' "$list" "$overlay" >"$tmp"; then
	exit 1
fi

mv "$tmp" "$overlay"
