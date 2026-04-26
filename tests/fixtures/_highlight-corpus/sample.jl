# Julia sample
module Sample

mutable struct Counter
    value::Int
    Counter() = new(0)
end

function increment!(c::Counter, n::Integer = 1)
    c.value += n
    return c.value
end

function count_bytes(path::AbstractString)::Int
    counter = Counter()
    open(path, "r") do f
        for line in eachline(f)
            if !isempty(line)
                increment!(counter, length(line))
            end
        end
    end
    return counter.value
end

function main(argv::Vector{String} = ARGS)
    path = isempty(argv) ? "input.txt" : argv[1]
    total = count_bytes(path)
    println("total bytes: $total")
    return 0
end

end  # module Sample

if abspath(PROGRAM_FILE) == @__FILE__
    Sample.main()
end
