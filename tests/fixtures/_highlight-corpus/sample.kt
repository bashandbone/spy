// Kotlin sample
package demo

import java.io.File

class Counter(private var value: Int = 0) {
    fun increment(n: Int = 1): Int {
        value += n
        return value
    }
    fun value(): Int = value
}

fun main() {
    val counter = Counter()
    File("input.txt").useLines { lines ->
        for (line in lines) {
            if (line.isNotEmpty()) {
                counter.increment(line.length)
            }
        }
    }
    println("total bytes: ${counter.value()}")
}
