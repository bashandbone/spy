# Elixir sample
defmodule Counter do
  @moduledoc "A small counter."

  defstruct value: 0

  @spec increment(t :: %__MODULE__{}, integer()) :: %__MODULE__{}
  def increment(%__MODULE__{value: v}, n \\ 1), do: %__MODULE__{value: v + n}

  @spec value(t :: %__MODULE__{}) :: integer()
  def value(%__MODULE__{value: v}), do: v
end

defmodule Sample do
  def count_bytes(path) do
    path
    |> File.stream!()
    |> Enum.reduce(%Counter{}, fn line, acc ->
      trimmed = String.trim_trailing(line, "\n")
      case trimmed do
        "" -> acc
        s  -> Counter.increment(acc, String.length(s))
      end
    end)
    |> Counter.value()
  end

  def main(args \\ []) do
    path = List.first(args, "input.txt")
    total = count_bytes(path)
    IO.puts("total bytes: #{total}")
  end
end

Sample.main(System.argv())
