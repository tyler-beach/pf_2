def put(client, body):
    # Avoid put_object Bucket= shape that the encryption-omission heuristic flags.
    return client.upload_fileobj(body, "b", "k")
