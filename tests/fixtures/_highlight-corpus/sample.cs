// C# sample
using System;
using System.IO;
using System.Linq;
using System.Threading.Tasks;

namespace Demo;

public sealed class Counter
{
    private int _value;

    public int Value => _value;

    public int Increment(int n = 1)
    {
        _value += n;
        return _value;
    }
}

internal static class Program
{
    public static async Task<int> Main()
    {
        var lines = await File.ReadAllLinesAsync("input.txt");
        var counter = new Counter();
        foreach (var line in lines.Where(l => !string.IsNullOrEmpty(l)))
        {
            counter.Increment(line.Length);
        }
        Console.WriteLine($"total bytes: {counter.Value}");
        return 0;
    }
}
