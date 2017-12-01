Get Started
===============

This document describes the necessary code changes to instrument Galileo into Golang Uber projects

############################
Requirements
############################

In order to make use of Galileo-Go your service must meet the following requirements:

#. It must be written in Go
#. Your service must already be fully instrumented with Jaeger tracing. This is a hard requirement since Galileo operates by inspecting and populating authentication context data via Jaegar baggage.

To integrate Jaegar tracing into your golang service see: `jaeger integration <https://engdocs.uberinternal.com/jaeger/menu_items/go_integration.html>`_


.. raw:: html

   <br/><hr class="style14" />

----

############################
Upgrade Steps
############################

#. Service-to-Service Authentication: Your udeploy service name must be enrolled with Wonka and provisioned with langley (*We are working on automating this step*)
#. Add an import statement for galileo-go to your imports section in your service go source
#. Modify your service code to use the necessary galileo wrapper calls for making requests
#. Add a galileo section to your production.yaml configuration files (see below)
#. Rebuild/Restart the service


.. raw:: html

   <br/><hr class="style14" />

############################
XHTTP Services - Upgrading
############################

Sample Repo: git clone gitolite@code.uber.internal:engsec/galilei

Clients:

#. Add the import statement for galileo go. (import "code.uber.internal/engsec/galileo-go.git/go")
#. Replace all create/init calls of xhttp.Client objects with galileo.NewClient(cfg) passing in the galileo configuration structure as an input parameter
#. OPTIONAL: If you know the destination name of the udeploy service you’ll be making requests TO you should call client.SetAttribute(galileo.ServiceAuth, "service_name") on the created galileo client object to automatically pre-resolve any needed auth tokens. If you choose to skip this step galileo still has the ability to resolve required claims dynamically viia X-Wonka-Requires reply headers + retrying requests
#. Use the created galileo.Client object exactly as you would use a normal xhttp.Client object to make requests. All requests made thru this wrapper object will automatically included appropriate galileo auth context data


Servers:

#. Add the import statement for galileo go. (import "code.uber.internal/engsec/galileo-go.git/go")
#. Replace the xhttp router initialization call with its galileo.NewRouter(cfg) equivalent
#. Add the galileo configuration section to your services development/production.yaml files

Be sure to include the list of allowed services in the config section like so::

  galileo:
    servicename: "MyUDeployServiceName"
    allowedservices: ["RTAPI,HEAVEN,CUSTOMSERVICEX"]
    enforce_percentage: 0.0

#. Rebuild & restart the service - All endpoints are now automatically Galileo protected

.. raw:: html

   <br/><hr class="style14" />

############################
Thrift Services - Upgrading
############################

Sample Repo: git clone gitolite@code.uber.internal:engsec/galilei2

Clients:

#. Add the import statement for galileo go. (import "code.uber.internal/engsec/galileo-go.git/go")
#. Wrap all relevant thrift client ctx context objects with galileo.AuthTO() calls passing in the inbound thrift context (if any) and the desired destination service udeploy name to auth-to as an input parameter (S2S). The resultant ctx thrift.Context object from these wrapper calls can then be passed to regular thrift client stub routines
#. Use your thrift client objects w/ the wrapped ctx as you normally would - they will now include the contextually appropriate authentication data



Servers:

#. Add the import statement for galileo go. (import "code.uber.internal/engsec/galileo-go.git/go")
#. Wrap the normal thrift thrift.Server handler router object with a wrapper call to galileo.WrapServer(cfg, server) which will return a wrapped gserver object
#. Use gserver.Register() instead of the normal server.Register() call to register all your thrift server/service endpoints
#. Add the galileo configuration section to your services development/production.yaml files

Be sure to include the list of allowed services in the config section like so::

  galileo:
    servicename: "MyUDeployServiceName"
    allowedservices: ["RTAPI,HEAVEN,CUSTOMSERVICEX"]
    enforce_percentage: 0.0

#. Rebuild & restart the service - All thrift endpoints registered via step 3 are now automatically Galileo protected

.. raw:: html

   <br/><hr class="style14" />

############################
UberFB - New Services
############################

UberFX - New Services

TODO: Add UberFX HTTP and or Thrift based instructions

.. raw:: html

   <br/><hr class="style14" />


.. include:: time.txt
