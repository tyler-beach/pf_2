import java.net.URL;
import java.util.Random;
import javax.crypto.Cipher;
import java.sql.Statement;

class RuntimeExecExtra {
  void bad(Statement stmt, String user) throws Exception {
    stmt.execute("SELECT * FROM t WHERE u=" + user);
    Cipher.getInstance("DES");
    new Random();
    (new URL(user)).openConnection();
  }
}
