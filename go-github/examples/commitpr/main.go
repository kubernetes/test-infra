# Community contribution robot

`In progress`

### Simple design ideas

> By recursing locally and checking files repeatedly, when errors are found,
Issues will be created in GitLab; users will check GitLab regularly and deal 
with Issues; By listening for the user's action on Issues, For example, `/pass` the 
document will be modified in the locally and create PR on GitHub. `/close` Issues will 
be closed directly. determines whether to create or close! Finally, the 
discovered error words are recorded in the library.


**Currently supported functionality**：

- [ ] Create project(GitLab)
- [x] Create issues(GitLab)
- [ ] Create wiki(GitLab)
- [ ] Listen issues(GitLab)
- [ ] Create a Pull Request(GitLab)
- [x] Get all pipes(GitLab)
- [x] Get all project(GitLab)
- [x] Create project tags(GitLab)
- [x] Create files in the repository(GitLab)
- [x] Get the language used by the project(GitLab)
- [x] Get all visible projects for an authenticated user(GitLab)
- [x] Create a New Repository(GitHub)
- [x] Create a New Pull Request(GitHub)

### Usage:

```shell
import "github.com/google/go-github/v21/github"	// with go modules enabled (GO111MODULE=on or outside GOPATH)
import "github.com/google/go-github/github"     // with go modules disabled
```

*To see an example, click [example](http://58.210.98.198:8888/Honorable_contributor/Clean_robot/tree/master/go-gitlab-master/examples).*