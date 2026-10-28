def put(s3, body):
    s3.put_object(Bucket="b", Key="k", Body=body, ACL="public-read")
    s3.put_object(Bucket="b", Key="k2", Body=body, ACL="public-read-write")
    s3.put_bucket_acl(Bucket="b", ACL="public-read")
    # IMDSv1
    url = "http://169.254.169.254/latest/meta-data"
    return url
