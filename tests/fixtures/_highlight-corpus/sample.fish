#!/usr/bin/env fish
# Fish shell sample

function sample_count_bytes --argument-names path
    set -l total 0
    if not test -f "$path"
        echo "input not found: $path" >&2
        return 1
    end
    while read -l line
        if test -n "$line"
            set total (math "$total + (string length \"$line\")")
        end
    end < "$path"
    echo $total
end

function main
    set -l path $argv[1]
    if test -z "$path"
        set path "input.txt"
    end
    set -l total (sample_count_bytes $path)
    if test $status -ne 0
        return $status
    end
    echo "total bytes: $total"
end

main $argv
