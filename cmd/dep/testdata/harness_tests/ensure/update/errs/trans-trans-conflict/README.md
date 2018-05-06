Have two direct dependencies point to two different versions of the same transient dependency.
 
root depends on deptestglideA at v0.4.0
root depends on deptestglideB at v0.3.0
deptestglideA depends on deptestglideC at v0.1.0
deptestglideB depends on deptestglideC at v0.2.0
 
Resolving should fail when trying to "ensure update deptestglideC" since no version meets constraints.
This test verifies that ensure reads constraints from dependencies.

"
[UBER]  Root project is "github.com/golang/notexist"
[UBER]   1 transitively valid internal packages
[UBER]   2 external packages imported from 2 projects
[UBER]  (0)   ✓ select (root)
[UBER]  (1)	? attempt github.com/ChinmayR/deptestglideA with 1 pkgs; at least 1 versions to try
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@v0.4.0
[UBER]  (1)	✓ select github.com/ChinmayR/deptestglideA@v0.4.0 w/1 pkgs
[UBER]  (2)	? attempt github.com/ChinmayR/deptestglideB with 1 pkgs; at least 1 versions to try
[UBER]  (2)	    try github.com/ChinmayR/deptestglideB@v0.3.0
[UBER]  (3)	✗   constraint ^0.2.0 on github.com/ChinmayR/deptestglideC disjoint with other dependers:
(3)	  ^0.1.0 from github.com/ChinmayR/deptestglideA@v0.4.0 (no overlap)
[UBER]  (2)	    try github.com/ChinmayR/deptestglideB@v0.5.0
[UBER]  (3)	✗   github.com/ChinmayR/deptestglideB@v0.5.0 not allowed by constraint 0.3.0:
(3)	    0.3.0 from (root)
[UBER]  (2)	    try github.com/ChinmayR/deptestglideB@v0.4.0
[UBER]  (3)	✗   github.com/ChinmayR/deptestglideB@v0.4.0 not allowed by constraint 0.3.0:
(3)	    0.3.0 from (root)
[UBER]  (2)	    try github.com/ChinmayR/deptestglideB@v0.2.0
[UBER]  (3)	✗   github.com/ChinmayR/deptestglideB@v0.2.0 not allowed by constraint 0.3.0:
(3)	    0.3.0 from (root)
[UBER]  (2)	    try github.com/ChinmayR/deptestglideB@v0.1.0
[UBER]  (3)	✗   github.com/ChinmayR/deptestglideB@v0.1.0 not allowed by constraint 0.3.0:
(3)	    0.3.0 from (root)
[UBER]  (2)	    try github.com/ChinmayR/deptestglideB@master
[UBER]  (3)	✗   github.com/ChinmayR/deptestglideB@master not allowed by constraint 0.3.0:
(3)	    0.3.0 from (root)
[UBER]  (2)	  ← no more versions of github.com/ChinmayR/deptestglideB to try; begin backtrack
[UBER]  (1)	← backtrack: no more versions of github.com/ChinmayR/deptestglideA to try
[UBER]  (1)	  ? continue github.com/ChinmayR/deptestglideA with 1 pkgs; 6 more versions to try
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@v0.6.0
[UBER]  (2)	✗   github.com/ChinmayR/deptestglideA@v0.6.0 not allowed by constraint 0.4.0:
(2)	    0.4.0 from (root)
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@v0.5.0
[UBER]  (2)	✗   github.com/ChinmayR/deptestglideA@v0.5.0 not allowed by constraint 0.4.0:
(2)	    0.4.0 from (root)
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@v0.3.0
[UBER]  (2)	✗   github.com/ChinmayR/deptestglideA@v0.3.0 not allowed by constraint 0.4.0:
(2)	    0.4.0 from (root)
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@v0.2.0
[UBER]  (2)	✗   github.com/ChinmayR/deptestglideA@v0.2.0 not allowed by constraint 0.4.0:
(2)	    0.4.0 from (root)
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@v0.1.0
[UBER]  (2)	✗   github.com/ChinmayR/deptestglideA@v0.1.0 not allowed by constraint 0.4.0:
(2)	    0.4.0 from (root)
[UBER]  (1)	    try github.com/ChinmayR/deptestglideA@master
[UBER]  (2)	✗   github.com/ChinmayR/deptestglideA@master not allowed by constraint 0.4.0:
(2)	    0.4.0 from (root)
[UBER]  (1)	← backtrack: no more versions of github.com/ChinmayR/deptestglideA to try
[UBER]    ✗ solving failed
[UBER]  
Solver wall times by segment:
[UBER]    b-source-exists: 1.06345837s
      b-list-pkgs:   3.28566ms
           b-gmal:  1.961681ms
          satisfy:   505.577µs
         new-atom:   345.466µs
        backtrack:   220.581µs
      select-root:   106.846µs
       b-pair-rev:    93.691µs
  b-list-versions:    78.544µs
      select-atom:    55.293µs
         unselect:    33.689µs
        b-matches:    30.149µs
    b-matches-any:    11.035µs
            other:    11.033µs
   b-pair-version:    10.445µs

  TOTAL: 1.07020806s

[INFO]  ensure Solve(): No versions of github.com/ChinmayR/deptestglideB met constraints:
	v0.3.0: Could not introduce github.com/ChinmayR/deptestglideB@v0.3.0, as it has a dependency on github.com/ChinmayR/deptestglideC with constraint ^0.2.0, which has no overlap with existing constraint ^0.1.0 from github.com/ChinmayR/deptestglideA@v0.4.0
	v0.5.0: Could not introduce github.com/ChinmayR/deptestglideB@v0.5.0, as it is not allowed by constraint 0.3.0 from project github.com/golang/notexist.
	v0.4.0: Could not introduce github.com/ChinmayR/deptestglideB@v0.4.0, as it is not allowed by constraint 0.3.0 from project github.com/golang/notexist.
	v0.2.0: Could not introduce github.com/ChinmayR/deptestglideB@v0.2.0, as it is not allowed by constraint 0.3.0 from project github.com/golang/notexist.
	v0.1.0: Could not introduce github.com/ChinmayR/deptestglideB@v0.1.0, as it is not allowed by constraint 0.3.0 from project github.com/golang/notexist.
	master: Could not introduce github.com/ChinmayR/deptestglideB@master, as it is not allowed by constraint 0.3.0 from project github.com/golang/notexist.

"