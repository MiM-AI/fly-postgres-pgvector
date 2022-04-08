package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"

	"github.com/fly-examples/postgres-ha/pkg/flypg"
	"github.com/fly-examples/postgres-ha/pkg/flypg/admin"
	"github.com/fly-examples/postgres-ha/pkg/render"
	"github.com/fly-examples/postgres-ha/pkg/util"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v4"
)

func Handler() http.Handler {
	r := chi.NewRouter()

	r.Route("/users", func(r chi.Router) {
		r.Get("/{name}", handleFindUser)
		r.Get("/list", handleListUsers)
		r.Post("/create", handleCreateUser)
		r.Delete("/delete/{name}", handleDeleteUser)
	})

	r.Route("/databases", func(r chi.Router) {
		r.Get("/list", handleListDatabases)
		r.Get("/{name}", handleFindDatabase)
		r.Post("/create", handleCreateDatabase)
		r.Delete("/delete/{name}", handleDeleteDatabase)
	})

	r.Route("/admin", func(r chi.Router) {
		// migrate commands under ./fyctl/cmd under an http handler insre
		r.Get("/failover", handleFailover)
		r.Get("/restart", handleRestart)
		r.Get("/settings", handleSettings)
		r.Post("/role", handleRole)

	})

	return r
}

func handleListDatabases(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	dbs, err := admin.ListDatabases(r.Context(), pg)
	if err != nil {
		render.Err(w, err)
		return
	}
	res := &Response{
		Result: dbs,
	}

	render.JSON(w, res, http.StatusOK)
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	users, err := admin.ListUsers(r.Context(), pg)
	if err != nil {
		render.Err(w, err)
		return
	}
	res := &Response{
		Result: users,
	}

	render.JSON(w, res, http.StatusOK)

}

func handleFindUser(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	name := chi.URLParam(r, "name")

	user, err := admin.FindUser(r.Context(), pg, name)
	if err != nil {
		render.Err(w, err)
		return
	}
	res := &Response{
		Result: user,
	}
	render.JSON(w, res, http.StatusOK)
}

func handleFindDatabase(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	name := chi.URLParam(r, "name")

	db, err := admin.FindDatabase(r.Context(), pg, name)
	if err != nil {
		render.Err(w, err)
		return
	}
	res := &Response{
		Result: db,
	}

	render.JSON(w, res, http.StatusOK)
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	var input createUserRequest

	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		render.Err(w, err)
		return
	}
	defer r.Body.Close()

	err = admin.CreateUser(r.Context(), pg, input.Username, input.Password)
	if err != nil {
		render.Err(w, err)
		return
	}

	if input.Database != "" {
		err = admin.GrantAccess(r.Context(), pg, input.Username, input.Database)
		if err != nil {
			render.Err(w, err)
			return
		}
	}

	if input.Superuser {
		err = admin.GrantSuperuser(r.Context(), pg, input.Username)
		if err != nil {
			render.Err(w, err)
			return
		}
	}
	res := &Response{
		Result: true,
	}

	render.JSON(w, res, http.StatusOK)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	name := chi.URLParam(r, "name")

	err = admin.DeleteUser(r.Context(), pg, name)
	if err != nil {
		render.Err(w, err)
		return
	}
	res := &Response{Result: true}
	render.JSON(w, res, http.StatusOK)
}

func handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	input := createDatabaseRequest{}

	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		render.Err(w, err)
		return
	}
	defer r.Body.Close()

	err = admin.CreateDatabase(r.Context(), pg, input.Name)
	if err != nil {
		render.Err(w, err)
		return
	}

	res := &Response{Result: true}

	render.JSON(w, res, http.StatusOK)
}

func handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	pg, close, err := getConnection(r.Context())
	if err != nil {
		render.Err(w, err)
		return
	}
	defer close()

	name := chi.URLParam(r, "name")

	err = admin.DeleteDatabase(r.Context(), pg, name)
	if err != nil {
		render.Err(w, err)
		return
	}
	res := &Response{Result: true}

	render.JSON(w, res, http.StatusOK)
}

func getConnection(ctx context.Context) (*pgx.Conn, func() error, error) {
	node, err := flypg.NewNode()
	if err != nil {
		return nil, nil, err
	}

	pg, err := node.NewProxyConnection(ctx)
	if err != nil {
		return nil, nil, err
	}
	close := func() error {
		return pg.Close(ctx)
	}

	return pg, close, nil
}

func handleFailover(w http.ResponseWriter, r *http.Request) {
	res := &Response{Result: true}

	render.JSON(w, res, http.StatusOK)
}

func handleRestart(w http.ResponseWriter, r *http.Request) {

	args := []string{"stolon", "pg_ctl", "-D", "/data/postgres", "restart"}

	cmd := exec.Command("gosu", args...)

	if err := cmd.Run(); err != nil {
		render.Err(w, err)
		return
	}

	if cmd.ProcessState.ExitCode() != 0 {
		err := fmt.Errorf(cmd.ProcessState.String())
		render.Err(w, err)
		return
	}

	res := &Response{Result: "Restart completed successfully"}

	render.JSON(w, res, http.StatusOK)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	res := &Response{Result: true}

	render.JSON(w, res, http.StatusOK)
}

func handleRole(w http.ResponseWriter, r *http.Request) {
	in := &checkRoleRequest{}

	err := json.NewDecoder(r.Body).Decode(&in)
	if err != nil {
		render.Err(w, err)
		return
	}
	endpoint := fmt.Sprintf("http://[%s]:5500/flycheck/role", in.Address)

	resp, err := http.Get(endpoint)
	if err != nil {
		render.Err(w, err)
	}

	if resp.StatusCode != http.StatusOK {
		render.Err(w, fmt.Errorf("role check failed %w", err))
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		util.WriteError(err)
	}

	role := strings.Trim(string(body), "\n")
	role = strings.Trim(role, "\"")

	res := &Response{Result: role}

	render.JSON(w, res, http.StatusOK)
}
