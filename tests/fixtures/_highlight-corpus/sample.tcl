# Tcl sample
package require Tcl 8.6

namespace eval ::sample {
    variable counter 0

    proc increment {n} {
        variable counter
        set counter [expr {$counter + $n}]
        return $counter
    }

    proc count_bytes {path} {
        variable counter
        set counter 0
        set fh [open $path r]
        try {
            while {[gets $fh line] >= 0} {
                set len [string length $line]
                if {$len > 0} {
                    increment $len
                }
            }
        } finally {
            close $fh
        }
        return $counter
    }
}

if {[info script] eq $::argv0} {
    set path [lindex $::argv 0]
    if {$path eq ""} { set path "input.txt" }
    set total [::sample::count_bytes $path]
    puts "total bytes: $total"
}
