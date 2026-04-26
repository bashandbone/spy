#!/usr/bin/env perl
# Perl sample
use strict;
use warnings;
use v5.30;

package Counter {
    sub new { my $class = shift; bless { value => 0 }, $class; }
    sub increment {
        my ($self, $n) = @_;
        $self->{value} += $n // 1;
        return $self->{value};
    }
    sub value { $_[0]->{value} }
}

sub main {
    my ($path) = @_;
    open(my $fh, '<', $path) or die "open $path: $!";
    my $counter = Counter->new();
    while (my $line = <$fh>) {
        chomp $line;
        $counter->increment(length $line) if length $line > 0;
    }
    close($fh);
    printf "total bytes: %d\n", $counter->value;
}

main($ARGV[0] // 'input.txt');
