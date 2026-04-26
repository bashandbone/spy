# Crystal sample
class Counter
  getter value : Int32

  def initialize
    @value = 0
  end

  def increment(n : Int32 = 1) : Int32
    @value += n
    @value
  end
end

def count_bytes(path : String) : Int32
  counter = Counter.new
  File.each_line(path) do |line|
    counter.increment(line.size) if line.size > 0
  end
  counter.value
end

path = ARGV[0]? || "input.txt"
puts "total bytes: #{count_bytes(path)}"
