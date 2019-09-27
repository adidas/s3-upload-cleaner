# S3 upload cleaner

This Go application cleans up incomplete or abandoned Multipart uploads from the S3 storage backend (started by CreateMultipartUpload S3 API call) that have been created more than 12h ago and still not completed or aborted. Some S3 storage provider implementations behaves wrongly if too many Multipart uploads are created and not completed or aborted, causing docker registry to fail when pushing layers. Some related docker registry issues exist, like:

https://github.com/docker/distribution/issues/1948


## License and Software Information
 
© adidas AG
 
adidas AG publishes this software and accompanied documentation (if any) subject to the terms of the MIT license with the aim of helping the community with our tools and libraries which we think can be also useful for other people. You will find a copy of the MIT license in the root folder of this package. All rights not explicitly granted to you under the MIT license remain the sole and exclusive property of adidas AG.
 
NOTICE: The software has been designed solely for the purpose of cleaning up incomplete Multipart uploads from S3 bucket storage. The software is NOT designed, tested or verified for productive use whatsoever, nor or for any use related to high risk environments, such as health care, highly or fully autonomous driving, power plants, or other critical infrastructures or services.
 
If you want to contact adidas regarding the software, you can mail us at _software.engineering@adidas.com_.
 
For further information open the [adidas terms and conditions](https://github.com/adidas/adidas-contribution-guidelines/wiki/Terms-and-conditions) page.

Disclaimer
----------

adidas is not responsible for the usage of this software for different purposes that the ones described in the use cases.

Usage
-----

`s3-upload-cleaner <endpoint> <bucket> <accessKey> <secreyAccessKey>`
  
Once run, it will remove abandoned uploads created more than 12h ago (see cleanupHours constant in the code).

Please note that this checks the *startedat* file inside the upload path to detect when the upload was started, but this **is specific to Docker registry**. 

If the S3 API is giving inconsistent responses (empty upload list, different list on every query), the script might need to be executed multiple times until the number of existing uploads is reduced.

It is recommended to run this as a scheduled job to perform a daily cleanup of incomplete uploads.
