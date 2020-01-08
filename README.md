# kube-networkpolicy-ensurer


This program watches for new namespaces and creates default NetworkPolicy for them. The policy allows incoming connections from pods of the same namespace:

```
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default
spec:
  ingress:
  - from:
    - podSelector: {}
  podSelector: {}
  policyTypes:
  - Ingress
```
