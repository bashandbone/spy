// Dart sample
import 'dart:io';

class Counter {
  int _value = 0;
  int increment([int n = 1]) {
    _value += n;
    return _value;
  }
  int get value => _value;
}

Future<void> main(List<String> args) async {
  final file = File(args.isNotEmpty ? args[0] : 'input.txt');
  if (!await file.exists()) {
    stderr.writeln('input file not found');
    exit(1);
  }
  final counter = Counter();
  await for (final line in file
      .openRead()
      .transform(const SystemEncoding().decoder)
      .transform(const LineSplitter())) {
    if (line.isNotEmpty) {
      counter.increment(line.length);
    }
  }
  print('total bytes: ${counter.value}');
}
