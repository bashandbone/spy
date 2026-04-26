<?php
// PHP sample
declare(strict_types=1);

namespace Demo;

final class Counter
{
    private int $value = 0;

    public function increment(int $n = 1): int
    {
        $this->value += $n;
        return $this->value;
    }

    public function value(): int
    {
        return $this->value;
    }
}

function main(): int
{
    $contents = file_get_contents('input.txt');
    if ($contents === false) {
        return 1;
    }
    $counter = new Counter();
    foreach (explode("\n", $contents) as $line) {
        if ($line !== '') {
            $counter->increment(strlen($line));
        }
    }
    printf("total bytes: %d\n", $counter->value());
    return 0;
}

exit(main());
