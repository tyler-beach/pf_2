import 'dart:io';
void main() {
  final input = Platform.environment['USER_COMMAND'] ?? '';
  Process.run('/bin/sh', ['-c', input]);
}
