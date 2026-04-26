# Ruby sample
# frozen_string_literal: true

class Counter
  attr_reader :value

  def initialize
    @value = 0
  end

  def increment(n = 1)
    @value += n
    @value
  end
end

def main
  counter = Counter.new
  File.foreach('input.txt') do |line|
    line = line.chomp
    counter.increment(line.length) unless line.empty?
  end
  puts "total bytes: #{counter.value}"
end

main if __FILE__ == $PROGRAM_NAME
