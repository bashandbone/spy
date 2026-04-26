// Groovy sample
package demo

class Counter {
    int value = 0

    int increment(int n = 1) {
        value += n
        return value
    }
}

static int countBytes(String path) {
    def counter = new Counter()
    new File(path).eachLine { line ->
        if (line) {
            counter.increment(line.length())
        }
    }
    return counter.value
}

if (this.binding.variables.containsKey('args')) {
    def path = args ? args[0] : 'input.txt'
    def total = countBytes(path)
    println "total bytes: ${total}"
}
