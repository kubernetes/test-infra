# IssueLabeler
This is intended to perform automatic labeling of github issues in the
kubernetes repository.

The code uses the python-flask framework to bring up a webapp that handles POST
requests containing two form fields: title and body.  

The webapp listens on all ips on on port 5000.

It passes those values to 2 pretrained machine learning models (SGDClassifier
from sklearn with hinge loss and l2 regularizer for those interested).  The
models return a team/<> label and a component/<> label.

In order to run the webapp, build the container using the following command.

$sudo docker build -no-cache -t <choose-image-name> . 

Then, run the container making sure to forward traffic to port 5000.

$docker run -p <port>:5000 <image name>
