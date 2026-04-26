-- Lua sample
local Counter = {}
Counter.__index = Counter

function Counter.new()
    return setmetatable({ value = 0 }, Counter)
end

function Counter:increment(n)
    self.value = self.value + (n or 1)
    return self.value
end

local function main(path)
    local f, err = io.open(path or "input.txt", "r")
    if not f then
        io.stderr:write(err, "\n")
        os.exit(1)
    end
    local counter = Counter.new()
    for line in f:lines() do
        if #line > 0 then
            counter:increment(#line)
        end
    end
    f:close()
    io.write(string.format("total bytes: %d\n", counter.value))
end

main(arg[1])
