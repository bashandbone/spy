// Scala sample
package demo

import scala.io.Source
import scala.util.Using

final class Counter(private var v: Int = 0):
  def increment(n: Int = 1): Int =
    v += n
    v
  def value: Int = v

@main def run(): Unit =
  val counter = Counter()
  Using.resource(Source.fromFile("input.txt")) { src =>
    src.getLines().filter(_.nonEmpty).foreach { line =>
      counter.increment(line.length)
    }
  }
  println(s"total bytes: ${counter.value}")
