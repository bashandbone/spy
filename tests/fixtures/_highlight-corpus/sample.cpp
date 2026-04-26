// C++ sample
#include <cstdint>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

namespace demo {

class Counter {
 public:
  Counter() : value_(0) {}
  int Increment(int n = 1) {
    value_ += n;
    return value_;
  }
  int value() const { return value_; }

 private:
  int value_;
};

}  // namespace demo

int main() {
  std::ifstream f("input.txt");
  std::ostringstream ss;
  ss << f.rdbuf();
  std::vector<std::string> lines;
  std::string line;
  std::istringstream iss(ss.str());
  while (std::getline(iss, line)) {
    if (!line.empty()) {
      lines.push_back(line);
    }
  }
  demo::Counter c;
  for (const auto& l : lines) {
    c.Increment(static_cast<int>(l.size()));
  }
  std::cout << "total bytes: " << c.value() << '\n';
  return 0;
}
