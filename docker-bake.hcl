variable "IMAGE_REGISTRY" {
  default = "ghcr.io/chantico-project/images"
}

variable "IMAGE_SUFFIX" {
  default = ""
}

variable "IMAGE_TAG" {
  default = "latest"
}

variable "IMAGE_SHA_TAG" {
  default = ""
}

group "default" {
  targets = ["controller", "visualisation-webapp", "snmp-mock", "vm-attribution-mock"]
}

target "controller" {
  context    = "."
  dockerfile = "Dockerfile"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
  cache-from = ["type=gha,scope=controller"]
  cache-to   = ["type=gha,scope=controller,mode=max,ignore-error=true"]
}

target "visualisation-webapp" {
  context    = "."
  dockerfile = "Dockerfile.webapp"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-visualisation-webapp${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-visualisation-webapp${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
  cache-from = ["type=gha,scope=visualisation-webapp"]
  cache-to   = ["type=gha,scope=visualisation-webapp,mode=max,ignore-error=true"]
}

target "snmp-mock" {
  context    = "."
  dockerfile = "Dockerfile.snmp-mock"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-snmp-mock${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-snmp-mock${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
  cache-from = ["type=gha,scope=snmp-mock"]
  cache-to   = ["type=gha,scope=snmp-mock,mode=max,ignore-error=true"]
}

target "vm-attribution-mock" {
  context    = "."
  dockerfile = "Dockerfile.vm-attribution-mock"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-vm-attribution-mock${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-vm-attribution-mock${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
  cache-from = ["type=gha,scope=vm-attribution-mock"]
  cache-to   = ["type=gha,scope=vm-attribution-mock,mode=max,ignore-error=true"]
}
