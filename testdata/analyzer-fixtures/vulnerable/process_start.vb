Imports System.Diagnostics
Module UnsafeProcess
    Sub Main()
        Dim command As String = Environment.GetEnvironmentVariable("USER_COMMAND")
        Process.Start(command)
    End Sub
End Module
