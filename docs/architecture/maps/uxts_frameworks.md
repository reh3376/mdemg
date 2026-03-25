UXTS|frameworks|v1|unified test specification frameworks
#name|scope|specs|runner|CI
UATS|HTTP contract|195 specs,224 variants,318 test cases|runner:active|CI:merge-blocking
UPTS|parser conformance|27s|runner:active|CI:merge-blocking
UDTS|gRPC contract|12 canonical,4 drafts|runner:active|CI:canonical-guard
UBTS|benchmark regression|3s,3 profiles|runner:active|CI:soft-fail
USTS|security|3 canonical,2 drafts|runner:active|CI:merge-blocking
UOBS|observability runtime|3 canonical,1 draft|runner:active|CI:soft-fail
UOTS|observability artifacts|5s|runner:active|CI:soft-fail
UNTS|hash verification|registry|runner:active|CI:merge-blocking
UETS|emergence eval:LLM concept-naming quality|8s|runner:active|CI:soft-fail
UITS|iterative-improvement:T1 encoding comprehension|11s|runner:active|CI:soft-fail
UVTS|semantic validation|1 canonical,1 draft|runner:active|CI:soft-fail
UAMS|auth methods|4s|runner:UNBUILT|CI:none→GAP-21:pending

STATUS-EXCEPTIONS:
  UAMS|runner:UNBUILT|only framework with no runner implementation→GAP-21
