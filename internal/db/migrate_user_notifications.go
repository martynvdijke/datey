package db

import (
	"context"
	"strings"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/pushsubscription"
	"github.com/datey/datey/ent/user"
)

// BackfillPushSubscriptions assigns orphan push subscriptions to the first admin.
func BackfillPushSubscriptions(ctx context.Context, client *ent.Client) error {
	admin, err := client.User.Query().Where(user.RoleEQ(user.RoleAdmin)).Order(ent.Asc(user.FieldID)).First(ctx)
	if err != nil {
		admin, err = client.User.Query().Order(ent.Asc(user.FieldID)).First(ctx)
		if err != nil {
			return nil
		}
	}
	orphans, err := client.PushSubscription.Query().Where(pushsubscription.Not(pushsubscription.HasUser())).All(ctx)
	if err != nil {
		// Fresh database: the table does not exist until Schema.Create runs.
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	for _, sub := range orphans {
		_ = client.PushSubscription.UpdateOneID(sub.ID).SetUserID(admin.ID).Exec(ctx)
	}
	return nil
}
