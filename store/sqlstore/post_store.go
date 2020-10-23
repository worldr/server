// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	sq "github.com/Masterminds/squirrel"
	"github.com/mattermost/mattermost-server/v5/einterfaces"
	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
	"github.com/mattermost/mattermost-server/v5/utils"
)

type SqlPostStore struct {
	SqlStore
	metrics           einterfaces.MetricsInterface
	maxPostSizeOnce   sync.Once
	maxPostSizeCached int
}

func (s *SqlPostStore) ClearCaches() {
}

func postSliceColumns() []string {
	return []string{
		"Id",
		"CreateAt",
		"UpdateAt",
		"EditAt",
		"DeleteAt",
		"IsPinned",
		"UserId",
		"ChannelId",
		"RootId",
		"ParentId",
		"ReplyToId",
		"OriginalId",
		"Message",
		"Type",
		"Props",
		"Hashtags",
		"Filenames",
		"FileIds",
		"HasReactions",
	}
}

func postToSlice(post *model.Post) []interface{} {
	return []interface{}{
		post.Id,
		post.CreateAt,
		post.UpdateAt,
		post.EditAt,
		post.DeleteAt,
		post.IsPinned,
		post.UserId,
		post.ChannelId,
		post.RootId,
		post.ParentId,
		post.ReplyToId,
		post.OriginalId,
		post.Message,
		post.Type,
		model.StringInterfaceToJson(post.Props),
		post.Hashtags,
		model.ArrayToJson(post.Filenames),
		model.ArrayToJson(post.FileIds),
		post.HasReactions,
	}
}

func newSqlPostStore(sqlStore SqlStore, metrics einterfaces.MetricsInterface) store.PostStore {
	s := &SqlPostStore{
		SqlStore:          sqlStore,
		metrics:           metrics,
		maxPostSizeCached: model.POST_MESSAGE_MAX_RUNES_V1,
	}

	for _, db := range sqlStore.GetAllConns() {
		table := db.AddTableWithName(model.Post{}, "Posts").SetKeys(false, "Id")
		table.ColMap("Id").SetMaxSize(26)
		table.ColMap("UserId").SetMaxSize(26)
		table.ColMap("ChannelId").SetMaxSize(26)
		table.ColMap("RootId").SetMaxSize(26)
		table.ColMap("ParentId").SetMaxSize(26)
		table.ColMap("ReplyToId").SetMaxSize(26)
		table.ColMap("OriginalId").SetMaxSize(26)
		table.ColMap("Message").SetMaxSize(model.POST_MESSAGE_MAX_BYTES_V2)
		table.ColMap("Type").SetMaxSize(26)
		table.ColMap("Hashtags").SetMaxSize(1000)
		table.ColMap("Props").SetMaxSize(8000)
		table.ColMap("Filenames").SetMaxSize(model.POST_FILENAMES_MAX_RUNES)
		table.ColMap("FileIds").SetMaxSize(150)
	}

	return s
}

func (s *SqlPostStore) createIndexesIfNotExists() {
	s.CreateIndexIfNotExists("idx_posts_update_at", "Posts", "UpdateAt")
	s.CreateIndexIfNotExists("idx_posts_create_at", "Posts", "CreateAt")
	s.CreateIndexIfNotExists("idx_posts_delete_at", "Posts", "DeleteAt")
	s.CreateIndexIfNotExists("idx_posts_channel_id", "Posts", "ChannelId")
	s.CreateIndexIfNotExists("idx_posts_root_id", "Posts", "RootId")
	s.CreateIndexIfNotExists("idx_posts_user_id", "Posts", "UserId")
	s.CreateIndexIfNotExists("idx_posts_is_pinned", "Posts", "IsPinned")

	s.CreateCompositeIndexIfNotExists("idx_posts_channel_id_update_at", "Posts", []string{"ChannelId", "UpdateAt"})
	s.CreateCompositeIndexIfNotExists("idx_posts_channel_id_delete_at_create_at", "Posts", []string{"ChannelId", "DeleteAt", "CreateAt"})

	s.CreateFullTextIndexIfNotExists("idx_posts_message_txt", "Posts", "Message")
	s.CreateFullTextIndexIfNotExists("idx_posts_hashtags_txt", "Posts", "Hashtags")
}

func (s *SqlPostStore) SaveMultiple(posts []*model.Post) ([]*model.Post, *model.AppError) {
	channelNewPosts := make(map[string]int)
	maxDateNewPosts := make(map[string]int64)
	rootIds := make(map[string]int)
	maxDateRootIds := make(map[string]int64)
	for _, post := range posts {
		if len(post.Id) > 0 {
			return nil, model.NewAppError("SqlPostStore.Save", "store.sql_post.save.existing.app_error", nil, "id="+post.Id, http.StatusBadRequest)
		}
		post.PreSave()
		maxPostSize := s.GetMaxPostSize()
		if err := post.IsValid(maxPostSize); err != nil {
			return nil, err
		}

		currentChannelCount, ok := channelNewPosts[post.ChannelId]
		if !ok {
			if post.IsJoinLeaveMessage() {
				channelNewPosts[post.ChannelId] = 0
			} else {
				channelNewPosts[post.ChannelId] = 1
			}
			maxDateNewPosts[post.ChannelId] = post.CreateAt
		} else {
			if !post.IsJoinLeaveMessage() {
				channelNewPosts[post.ChannelId] = currentChannelCount + 1
			}
			if post.CreateAt > maxDateNewPosts[post.ChannelId] {
				maxDateNewPosts[post.ChannelId] = post.CreateAt
			}
		}

		if len(post.RootId) == 0 {
			continue
		}

		currentRootCount, ok := rootIds[post.RootId]
		if !ok {
			rootIds[post.RootId] = 1
			maxDateRootIds[post.RootId] = post.CreateAt
		} else {
			rootIds[post.RootId] = currentRootCount + 1
			if post.CreateAt > maxDateRootIds[post.RootId] {
				maxDateRootIds[post.RootId] = post.CreateAt
			}
		}
	}

	query := s.getQueryBuilder().Insert("Posts").Columns(postSliceColumns()...)
	for _, post := range posts {
		query = query.Values(postToSlice(post)...)
	}
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.Save", "store.sql_post.save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	if _, err := s.GetMaster().Exec(sql, args...); err != nil {
		return nil, model.NewAppError("SqlPostStore.Save", "store.sql_post.save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	for channelId, count := range channelNewPosts {
		if _, err := s.GetMaster().Exec("UPDATE Channels SET LastPostAt = GREATEST(:LastPostAt, LastPostAt), TotalMsgCount = TotalMsgCount + :Count WHERE Id = :ChannelId", map[string]interface{}{"LastPostAt": maxDateNewPosts[channelId], "ChannelId": channelId, "Count": count}); err != nil {
			mlog.Error("Error updating Channel LastPostAt.", mlog.Err(err))
		}
	}

	for rootId := range rootIds {
		if _, err := s.GetMaster().Exec("UPDATE Posts SET UpdateAt = :UpdateAt WHERE Id = :RootId", map[string]interface{}{"UpdateAt": maxDateRootIds[rootId], "RootId": rootId}); err != nil {
			mlog.Error("Error updating Post UpdateAt.", mlog.Err(err))
		}
	}

	for _, post := range posts {
		if len(post.RootId) == 0 {
			count, ok := rootIds[post.Id]
			if ok {
				post.ReplyCount += int64(count)
			}
		}
	}

	return posts, nil
}

func (s *SqlPostStore) Save(post *model.Post) (*model.Post, *model.AppError) {
	posts, err := s.SaveMultiple([]*model.Post{post})
	if err != nil {
		return nil, err
	}
	return posts[0], nil
}

func (s *SqlPostStore) Update(newPost *model.Post, oldPost *model.Post) (*model.Post, *model.AppError) {
	newPost.UpdateAt = model.GetMillis()
	newPost.PreCommit()

	oldPost.DeleteAt = newPost.UpdateAt
	oldPost.UpdateAt = newPost.UpdateAt
	oldPost.OriginalId = oldPost.Id
	oldPost.Id = model.NewId()
	oldPost.PreCommit()

	maxPostSize := s.GetMaxPostSize()

	if err := newPost.IsValid(maxPostSize); err != nil {
		return nil, err
	}

	if _, err := s.GetMaster().Update(newPost); err != nil {
		return nil, model.NewAppError("SqlPostStore.Update", "store.sql_post.update.app_error", nil, "id="+newPost.Id+", "+err.Error(), http.StatusInternalServerError)
	}

	time := model.GetMillis()
	s.GetMaster().Exec("UPDATE Channels SET LastPostAt = :LastPostAt  WHERE Id = :ChannelId AND LastPostAt < :LastPostAt", map[string]interface{}{"LastPostAt": time, "ChannelId": newPost.ChannelId})

	if len(newPost.RootId) > 0 {
		s.GetMaster().Exec("UPDATE Posts SET UpdateAt = :UpdateAt WHERE Id = :RootId AND UpdateAt < :UpdateAt", map[string]interface{}{"UpdateAt": time, "RootId": newPost.RootId})
	}

	// mark the old post as deleted
	s.GetMaster().Insert(oldPost)

	return newPost, nil
}

func (s *SqlPostStore) OverwriteMultiple(posts []*model.Post) ([]*model.Post, *model.AppError) {
	updateAt := model.GetMillis()
	maxPostSize := s.GetMaxPostSize()
	for _, post := range posts {
		post.UpdateAt = updateAt
		if appErr := post.IsValid(maxPostSize); appErr != nil {
			return nil, appErr
		}
	}

	tx, err := s.GetMaster().Begin()
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.Overwrite", "store.sql_post.overwrite.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	for _, post := range posts {
		if _, err = tx.Update(post); err != nil {
			txErr := tx.Rollback()
			if txErr != nil {
				return nil, model.NewAppError("SqlPostStore.Overwrite", "store.sql_post.overwrite.app_error", nil, txErr.Error(), http.StatusInternalServerError)
			}

			return nil, model.NewAppError("SqlPostStore.Overwrite", "store.sql_post.overwrite.app_error", nil, "id="+post.Id+", "+err.Error(), http.StatusInternalServerError)
		}
	}
	err = tx.Commit()
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.Overwrite", "store.sql_post.overwrite.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	return posts, nil
}

func (s *SqlPostStore) Overwrite(post *model.Post) (*model.Post, *model.AppError) {
	posts, err := s.OverwriteMultiple([]*model.Post{post})
	if err != nil {
		return nil, err
	}

	return posts[0], nil
}

func (s *SqlPostStore) GetFlaggedPosts(userId string, offset int, limit int) (*model.PostList, *model.AppError) {
	pl := model.NewPostList()

	var posts []*model.Post
	if _, err := s.GetReplica().Select(&posts, "SELECT *, (SELECT count(Posts.Id) FROM Posts WHERE Posts.RootId = p.Id AND Posts.DeleteAt = 0) as ReplyCount FROM Posts p WHERE Id IN (SELECT Name FROM Preferences WHERE UserId = :UserId AND Category = :Category) AND DeleteAt = 0 ORDER BY CreateAt DESC LIMIT :Limit OFFSET :Offset", map[string]interface{}{"UserId": userId, "Category": model.PREFERENCE_CATEGORY_FLAGGED_POST, "Offset": offset, "Limit": limit}); err != nil {
		return nil, model.NewAppError("SqlPostStore.GetFlaggedPosts", "store.sql_post.get_flagged_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	for _, post := range posts {
		pl.AddPost(post)
		pl.AddOrder(post.Id)
	}

	return pl, nil
}

func (s *SqlPostStore) GetFlaggedPostsForTeam(userId, teamId string, offset int, limit int) (*model.PostList, *model.AppError) {
	pl := model.NewPostList()

	var posts []*model.Post

	query := `
            SELECT
                A.*, (SELECT count(Posts.Id) FROM Posts WHERE Posts.RootId = A.Id AND Posts.DeleteAt = 0) as ReplyCount
            FROM
                (SELECT
                    *
                FROM
                    Posts
                WHERE
                    Id
                IN
                    (SELECT
                        Name
                    FROM
                        Preferences
                    WHERE
                        UserId = :UserId
                        AND Category = :Category)
                        AND DeleteAt = 0
                ) as A
            INNER JOIN Channels as B
                ON B.Id = A.ChannelId
            WHERE B.TeamId = :TeamId OR B.TeamId = ''
            ORDER BY CreateAt DESC
            LIMIT :Limit OFFSET :Offset`

	if _, err := s.GetReplica().Select(&posts, query, map[string]interface{}{"UserId": userId, "Category": model.PREFERENCE_CATEGORY_FLAGGED_POST, "Offset": offset, "Limit": limit, "TeamId": teamId}); err != nil {
		return nil, model.NewAppError("SqlPostStore.GetFlaggedPostsForTeam", "store.sql_post.get_flagged_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	for _, post := range posts {
		pl.AddPost(post)
		pl.AddOrder(post.Id)
	}

	return pl, nil
}

func (s *SqlPostStore) GetFlaggedPostsForChannel(userId, channelId string, offset int, limit int) (*model.PostList, *model.AppError) {
	pl := model.NewPostList()

	var posts []*model.Post
	query := `
		SELECT
			*, (SELECT count(Posts.Id) FROM Posts WHERE Posts.RootId = p.Id AND Posts.DeleteAt = 0) as ReplyCount
		FROM Posts p
		WHERE
			Id IN (SELECT Name FROM Preferences WHERE UserId = :UserId AND Category = :Category)
			AND ChannelId = :ChannelId
			AND DeleteAt = 0
		ORDER BY CreateAt DESC
		LIMIT :Limit OFFSET :Offset`

	if _, err := s.GetReplica().Select(&posts, query, map[string]interface{}{"UserId": userId, "Category": model.PREFERENCE_CATEGORY_FLAGGED_POST, "ChannelId": channelId, "Offset": offset, "Limit": limit}); err != nil {
		return nil, model.NewAppError("SqlPostStore.GetFlaggedPostsForChannel", "store.sql_post.get_flagged_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	for _, post := range posts {
		pl.AddPost(post)
		pl.AddOrder(post.Id)
	}

	return pl, nil
}

func (s *SqlPostStore) Get(id string, skipFetchThreads bool) (*model.PostList, *model.AppError) {
	pl := model.NewPostList()

	if len(id) == 0 {
		return nil, model.NewAppError("SqlPostStore.GetPost", "store.sql_post.get.app_error", nil, "id="+id, http.StatusBadRequest)
	}

	var post model.Post
	postFetchQuery := "SELECT p.*, (SELECT count(Posts.Id) FROM Posts WHERE Posts.RootId = p.Id AND Posts.DeleteAt = 0) as ReplyCount FROM Posts p WHERE p.Id = :Id AND p.DeleteAt = 0"
	err := s.GetReplica().SelectOne(&post, postFetchQuery, map[string]interface{}{"Id": id})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPost", "store.sql_post.get.app_error", nil, "id="+id+err.Error(), http.StatusNotFound)
	}
	pl.AddPost(&post)
	pl.AddOrder(id)
	if !skipFetchThreads {
		rootId := post.RootId

		if rootId == "" {
			rootId = post.Id
		}

		if len(rootId) == 0 {
			return nil, model.NewAppError("SqlPostStore.GetPost", "store.sql_post.get.app_error", nil, "root_id="+rootId, http.StatusInternalServerError)
		}

		var posts []*model.Post
		_, err = s.GetReplica().Select(&posts, "SELECT *, (SELECT count(Id) FROM Posts WHERE RootId = p.Id AND Posts.DeleteAt = 0) as ReplyCount FROM Posts p WHERE (Id = :Id OR RootId = :RootId) AND DeleteAt = 0", map[string]interface{}{"Id": rootId, "RootId": rootId})
		if err != nil {
			return nil, model.NewAppError("SqlPostStore.GetPost", "store.sql_post.get.app_error", nil, "root_id="+rootId+err.Error(), http.StatusInternalServerError)
		}

		for _, p := range posts {
			pl.AddPost(p)
			pl.AddOrder(p.Id)
		}
	}
	return pl, nil
}

func (s *SqlPostStore) GetSingle(id string) (*model.Post, *model.AppError) {
	var post model.Post
	err := s.GetReplica().SelectOne(&post, "SELECT * FROM Posts WHERE Id = :Id AND DeleteAt = 0", map[string]interface{}{"Id": id})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetSingle", "store.sql_post.get.app_error", nil, "id="+id+err.Error(), http.StatusNotFound)
	}
	return &post, nil
}

type etagPosts struct {
	Id       string
	UpdateAt int64
}

func (s *SqlPostStore) InvalidateLastPostTimeCache(channelId string) {
}

func (s *SqlPostStore) GetEtag(channelId string, allowFromCache bool) string {
	var et etagPosts
	err := s.GetReplica().SelectOne(&et, "SELECT Id, UpdateAt FROM Posts WHERE ChannelId = :ChannelId ORDER BY UpdateAt DESC LIMIT 1", map[string]interface{}{"ChannelId": channelId})
	var result string
	if err != nil {
		result = fmt.Sprintf("%v.%v", model.CurrentVersion, model.GetMillis())
	} else {
		result = fmt.Sprintf("%v.%v", model.CurrentVersion, et.UpdateAt)
	}

	return result
}

func (s *SqlPostStore) Delete(postId string, time int64, deleteByID string) *model.AppError {

	appErr := func(errMsg string) *model.AppError {
		return model.NewAppError("SqlPostStore.Delete", "store.sql_post.delete.app_error", nil, "id="+postId+", err="+errMsg, http.StatusInternalServerError)
	}

	var post model.Post
	err := s.GetReplica().SelectOne(&post, "SELECT * FROM Posts WHERE Id = :Id AND DeleteAt = 0", map[string]interface{}{"Id": postId})
	if err != nil {
		return appErr(err.Error())
	}

	post.Props[model.POST_PROPS_DELETE_BY] = deleteByID

	_, err = s.GetMaster().Exec("UPDATE Posts SET DeleteAt = :DeleteAt, UpdateAt = :UpdateAt, Props = :Props WHERE Id = :Id OR RootId = :RootId", map[string]interface{}{"DeleteAt": time, "UpdateAt": time, "Id": postId, "RootId": postId, "Props": model.StringInterfaceToJson(post.Props)})
	if err != nil {
		return appErr(err.Error())
	}

	return nil
}

func (s *SqlPostStore) permanentDelete(postId string) *model.AppError {
	_, err := s.GetMaster().Exec("DELETE FROM Posts WHERE Id = :Id OR RootId = :RootId", map[string]interface{}{"Id": postId, "RootId": postId})
	if err != nil {
		return model.NewAppError("SqlPostStore.Delete", "store.sql_post.permanent_delete.app_error", nil, "id="+postId+", err="+err.Error(), http.StatusInternalServerError)
	}
	return nil
}

func (s *SqlPostStore) permanentDeleteAllCommentByUser(userId string) *model.AppError {
	_, err := s.GetMaster().Exec("DELETE FROM Posts WHERE UserId = :UserId AND RootId != ''", map[string]interface{}{"UserId": userId})
	if err != nil {
		return model.NewAppError("SqlPostStore.permanentDeleteAllCommentByUser", "store.sql_post.permanent_delete_all_comments_by_user.app_error", nil, "userId="+userId+", err="+err.Error(), http.StatusInternalServerError)
	}
	return nil
}

func (s *SqlPostStore) PermanentDeleteByUser(userId string) *model.AppError {
	// First attempt to delete all the comments for a user
	if err := s.permanentDeleteAllCommentByUser(userId); err != nil {
		return err
	}

	// Now attempt to delete all the root posts for a user. This will also
	// delete all the comments for each post
	found := true
	count := 0

	for found {
		var ids []string
		_, err := s.GetMaster().Select(&ids, "SELECT Id FROM Posts WHERE UserId = :UserId LIMIT 1000", map[string]interface{}{"UserId": userId})
		if err != nil {
			return model.NewAppError("SqlPostStore.PermanentDeleteByUser.select", "store.sql_post.permanent_delete_by_user.app_error", nil, "userId="+userId+", err="+err.Error(), http.StatusInternalServerError)
		}

		found = false
		for _, id := range ids {
			found = true
			if err := s.permanentDelete(id); err != nil {
				return err
			}
		}

		// This is a fail safe, give up if more than 10k messages
		count++
		if count >= 10 {
			return model.NewAppError("SqlPostStore.PermanentDeleteByUser.toolarge", "store.sql_post.permanent_delete_by_user.too_many.app_error", nil, "userId="+userId, http.StatusInternalServerError)
		}
	}

	return nil
}

func (s *SqlPostStore) PermanentDeleteByChannel(channelId string) *model.AppError {
	if _, err := s.GetMaster().Exec("DELETE FROM Posts WHERE ChannelId = :ChannelId", map[string]interface{}{"ChannelId": channelId}); err != nil {
		return model.NewAppError("SqlPostStore.PermanentDeleteByChannel", "store.sql_post.permanent_delete_by_channel.app_error", nil, "channel_id="+channelId+", "+err.Error(), http.StatusInternalServerError)
	}
	return nil
}

func (s *SqlPostStore) GetPosts(options model.GetPostsOptions, _ bool) (*model.PostList, *model.AppError) {
	if options.PerPage > 1000 {
		return nil, model.NewAppError("SqlPostStore.GetLinearPosts", "store.sql_post.get_posts.app_error", nil, "channelId="+options.ChannelId, http.StatusBadRequest)
	}
	offset := options.PerPage * options.Page

	rpc := make(chan store.StoreResult, 1)
	go func() {
		posts, err := s.getRootPosts(options.ChannelId, offset, options.PerPage, options.SkipFetchThreads)
		rpc <- store.StoreResult{Data: posts, Err: err}
		close(rpc)
	}()
	cpc := make(chan store.StoreResult, 1)
	go func() {
		posts, err := s.getParentsPosts(options.ChannelId, offset, options.PerPage, options.SkipFetchThreads)
		cpc <- store.StoreResult{Data: posts, Err: err}
		close(cpc)
	}()

	var err *model.AppError
	list := model.NewPostList()

	rpr := <-rpc
	if rpr.Err != nil {
		return nil, rpr.Err
	}

	cpr := <-cpc
	if cpr.Err != nil {
		return nil, cpr.Err
	}

	posts := rpr.Data.([]*model.Post)
	parents := cpr.Data.([]*model.Post)

	for _, p := range posts {
		list.AddPost(p)
		list.AddOrder(p.Id)
	}

	for _, p := range parents {
		list.AddPost(p)
	}

	list.MakeNonNil()

	return list, err
}

func (s *SqlPostStore) GetPostsSince(options model.GetPostsSinceOptions, allowFromCache bool) (*model.PostList, *model.AppError) {
	var posts []*model.Post

	replyCountQuery1 := ""
	replyCountQuery2 := ""
	if options.SkipFetchThreads {
		replyCountQuery1 = `, (SELECT COUNT(Posts.Id) FROM Posts WHERE p1.RootId = '' AND Posts.RootId = p1.Id AND Posts.DeleteAt = 0) as ReplyCount`
		replyCountQuery2 = `, (SELECT COUNT(Posts.Id) FROM Posts WHERE p2.RootId = '' AND Posts.RootId = p2.Id AND Posts.DeleteAt = 0) as ReplyCount`
	}
	var query string

	// union of IDs and then join to get full posts is faster in mysql
	if s.DriverName() == model.DATABASE_DRIVER_MYSQL {
		query = `SELECT *` + replyCountQuery1 + ` FROM Posts p1 JOIN (
			(SELECT
              Id
			  FROM
				  Posts p2
			  WHERE
				  (UpdateAt > :Time
					  AND ChannelId = :ChannelId)
				  LIMIT 1000)
			  UNION
				  (SELECT
					  Id
				  FROM
					  Posts p3
				  WHERE
					  Id
				  IN
					  (SELECT * FROM (SELECT
						  RootId
					  FROM
						  Posts
					  WHERE
						  UpdateAt > :Time
							  AND ChannelId = :ChannelId
					  LIMIT 1000) temp_tab))
			) j ON p1.Id = j.Id
          ORDER BY CreateAt DESC`
	} else if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		query = `
			(SELECT
                       *` + replyCountQuery1 + `
               FROM
                       Posts p1
               WHERE
                       (UpdateAt > :Time
                               AND ChannelId = :ChannelId)
                       LIMIT 1000)
               UNION
                       (SELECT
                           *` + replyCountQuery2 + `
                       FROM
                           Posts p2
                       WHERE
                           Id
                       IN
                           (SELECT * FROM (SELECT
                               RootId
                           FROM
                               Posts
                           WHERE
                               UpdateAt > :Time
                                               AND ChannelId = :ChannelId
                               LIMIT 1000) temp_tab))
               ORDER BY CreateAt DESC`
	}
	_, err := s.GetReplica().Select(&posts, query, map[string]interface{}{"ChannelId": options.ChannelId, "Time": options.Time})

	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPostsSince", "store.sql_post.get_posts_since.app_error", nil, "channelId="+options.ChannelId+err.Error(), http.StatusInternalServerError)
	}

	list := model.NewPostList()

	for _, p := range posts {
		list.AddPost(p)
		if p.UpdateAt > options.Time {
			list.AddOrder(p.Id)
		}
	}

	return list, nil
}

func (s *SqlPostStore) GetPostsBefore(options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	return s.getPostsAround(true, options)
}

func (s *SqlPostStore) GetPostsAfter(options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	return s.getPostsAround(false, options)
}

func (s *SqlPostStore) getPostsAround(before bool, options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	offset := options.Page * options.PerPage
	var posts, parents []*model.Post

	var direction string
	var sort string
	if before {
		direction = "<"
		sort = "DESC"
	} else {
		direction = ">"
		sort = "ASC"
	}
	replyCountSubQuery := s.getQueryBuilder().Select("COUNT(Posts.Id)").From("Posts").Where(sq.Expr("p.RootId = '' AND RootId = p.Id AND DeleteAt = 0"))
	query := s.getQueryBuilder().Select("p.*")
	if options.SkipFetchThreads {
		query = query.Column(sq.Alias(replyCountSubQuery, "ReplyCount"))
	}
	query = query.From("Posts p").
		Where(sq.And{
			sq.Expr(`CreateAt `+direction+` (SELECT CreateAt FROM Posts WHERE Id = ?)`, options.PostId),
			sq.Eq{"ChannelId": options.ChannelId},
			sq.Eq{"DeleteAt": int(0)},
		}).
		OrderBy("CreateAt " + sort).
		Limit(uint64(options.PerPage)).
		Offset(uint64(offset))

	queryString, args, err := query.ToSql()

	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPostContext", "store.sql_post.get_posts_around.get.app_error", nil, "channelId="+options.ChannelId+err.Error(), http.StatusInternalServerError)
	}
	_, err = s.GetMaster().Select(&posts, queryString, args...)
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPostContext", "store.sql_post.get_posts_around.get.app_error", nil, "channelId="+options.ChannelId+err.Error(), http.StatusInternalServerError)
	}

	if len(posts) > 0 {
		rootIds := []string{}
		for _, post := range posts {
			rootIds = append(rootIds, post.Id)
			if post.RootId != "" {
				rootIds = append(rootIds, post.RootId)
			}
		}
		rootQuery := s.getQueryBuilder().Select("p.*")
		idQuery := sq.Or{
			sq.Eq{"Id": rootIds},
		}
		if options.SkipFetchThreads {
			rootQuery = rootQuery.Column(sq.Alias(replyCountSubQuery, "ReplyCount"))
		} else {
			idQuery = append(idQuery, sq.Eq{"RootId": rootIds}) // preserve original behaviour
		}

		rootQuery = rootQuery.From("Posts p").
			Where(sq.And{
				idQuery,
				sq.Eq{"ChannelId": options.ChannelId},
				sq.Eq{"DeleteAt": 0},
			}).
			OrderBy("CreateAt DESC")

		rootQueryString, rootArgs, err := rootQuery.ToSql()

		if err != nil {
			return nil, model.NewAppError("SqlPostStore.GetPostContext", "store.sql_post.get_posts_around.get_parent.app_error", nil, "channelId="+options.ChannelId+err.Error(), http.StatusInternalServerError)
		}
		_, err = s.GetMaster().Select(&parents, rootQueryString, rootArgs...)
		if err != nil {
			return nil, model.NewAppError("SqlPostStore.GetPostContext", "store.sql_post.get_posts_around.get_parent.app_error", nil, "channelId="+options.ChannelId+err.Error(), http.StatusInternalServerError)
		}
	}

	list := model.NewPostList()

	// We need to flip the order if we selected backwards
	if before {
		for _, p := range posts {
			list.AddPost(p)
			list.AddOrder(p.Id)
		}
	} else {
		l := len(posts)
		for i := range posts {
			list.AddPost(posts[l-i-1])
			list.AddOrder(posts[l-i-1].Id)
		}
	}

	for _, p := range parents {
		list.AddPost(p)
	}

	return list, nil
}

func (s *SqlPostStore) GetPostIdBeforeTime(channelId string, time int64) (string, *model.AppError) {
	return s.getPostIdAroundTime(channelId, time, true)
}

func (s *SqlPostStore) GetPostIdAfterTime(channelId string, time int64) (string, *model.AppError) {
	return s.getPostIdAroundTime(channelId, time, false)
}

func (s *SqlPostStore) getPostIdAroundTime(channelId string, time int64, before bool) (string, *model.AppError) {
	var direction sq.Sqlizer
	var sort string
	if before {
		direction = sq.Lt{"CreateAt": time}
		sort = "DESC"
	} else {
		direction = sq.Gt{"CreateAt": time}
		sort = "ASC"
	}

	query := s.getQueryBuilder().
		Select("Id").
		From("Posts").
		Where(sq.And{
			direction,
			sq.Eq{"ChannelId": channelId},
			sq.Eq{"DeleteAt": int(0)},
		}).
		OrderBy("CreateAt " + sort).
		Limit(1)

	queryString, args, err := query.ToSql()
	if err != nil {
		return "", model.NewAppError("SqlPostStore.getPostIdAroundTime", "store.sql_post.get_post_id_around.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	var postId string
	if err := s.GetMaster().SelectOne(&postId, queryString, args...); err != nil {
		if err != sql.ErrNoRows {
			return "", model.NewAppError("SqlPostStore.getPostIdAroundTime", "store.sql_post.get_post_id_around.app_error", nil, "channelId="+channelId+err.Error(), http.StatusInternalServerError)
		}
	}

	return postId, nil
}

func (s *SqlPostStore) GetPostAfterTime(channelId string, time int64) (*model.Post, *model.AppError) {
	query := s.getQueryBuilder().
		Select("*").
		From("Posts").
		Where(sq.And{
			sq.Gt{"CreateAt": time},
			sq.Eq{"ChannelId": channelId},
			sq.Eq{"DeleteAt": int(0)},
		}).
		OrderBy("CreateAt ASC").
		Limit(1)

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPostAfterTime", "store.sql_post.get_post_after_time.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	var post *model.Post
	if err := s.GetMaster().SelectOne(&post, queryString, args...); err != nil {
		if err != sql.ErrNoRows {
			return nil, model.NewAppError("SqlPostStore.GetPostAfterTime", "store.sql_post.get_post_after_time.app_error", nil, "channelId="+channelId+err.Error(), http.StatusInternalServerError)
		}
	}

	return post, nil
}

func (s *SqlPostStore) getRootPosts(channelId string, offset int, limit int, skipFetchThreads bool) ([]*model.Post, *model.AppError) {
	var posts []*model.Post
	var fetchQuery string
	if skipFetchThreads {
		fetchQuery = "SELECT p.*, (SELECT COUNT(Posts.Id) FROM Posts WHERE p.RootId = '' AND Posts.RootId = p.Id AND Posts.DeleteAt = 0) as ReplyCount FROM Posts p WHERE ChannelId = :ChannelId AND DeleteAt = 0 ORDER BY CreateAt DESC LIMIT :Limit OFFSET :Offset"
	} else {
		fetchQuery = "SELECT * FROM Posts WHERE ChannelId = :ChannelId AND DeleteAt = 0 ORDER BY CreateAt DESC LIMIT :Limit OFFSET :Offset"
	}
	_, err := s.GetReplica().Select(&posts, fetchQuery, map[string]interface{}{"ChannelId": channelId, "Offset": offset, "Limit": limit})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetLinearPosts", "store.sql_post.get_root_posts.app_error", nil, "channelId="+channelId+err.Error(), http.StatusInternalServerError)
	}
	return posts, nil
}

func (s *SqlPostStore) getParentsPosts(channelId string, offset int, limit int, skipFetchThreads bool) ([]*model.Post, *model.AppError) {
	if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		return s.getParentsPostsPostgreSQL(channelId, offset, limit, skipFetchThreads)
	}

	// query parent Ids first
	var roots []*struct {
		RootId string
	}
	rootQuery := `
		SELECT DISTINCT
			q.RootId
		FROM
			(SELECT
				RootId
			FROM
				Posts
			WHERE
				ChannelId = :ChannelId
					AND DeleteAt = 0
			ORDER BY CreateAt DESC
			LIMIT :Limit OFFSET :Offset) q
		WHERE q.RootId != ''`

	_, err := s.GetReplica().Select(&roots, rootQuery, map[string]interface{}{"ChannelId": channelId, "Offset": offset, "Limit": limit})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetLinearPosts", "store.sql_post.get_parents_posts.app_error", nil, "channelId="+channelId+" err="+err.Error(), http.StatusInternalServerError)
	}
	if len(roots) == 0 {
		return nil, nil
	}
	params := make(map[string]interface{})
	placeholders := make([]string, len(roots))
	for idx, r := range roots {
		key := fmt.Sprintf(":Root%v", idx)
		params[key[1:]] = r.RootId
		placeholders[idx] = key
	}
	placeholderString := strings.Join(placeholders, ", ")
	params["ChannelId"] = channelId
	replyCountQuery := ""
	whereStatement := "p.Id IN (" + placeholderString + ")"
	if skipFetchThreads {
		replyCountQuery = `, (SELECT COUNT(Posts.Id) FROM Posts WHERE p.RootId = '' AND Posts.RootId = p.Id AND Posts.DeleteAt = 0) as ReplyCount`
	} else {
		whereStatement += " OR p.RootId IN (" + placeholderString + ")"
	}
	var posts []*model.Post
	_, err = s.GetReplica().Select(&posts, `
		SELECT p.*`+replyCountQuery+`
		FROM
			Posts p
		WHERE
			(`+whereStatement+`)
				AND ChannelId = :ChannelId
				AND DeleteAt = 0
		ORDER BY CreateAt`,
		params)
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetLinearPosts", "store.sql_post.get_parents_posts.app_error", nil, "channelId="+channelId+" err="+err.Error(), http.StatusInternalServerError)
	}
	return posts, nil
}

func (s *SqlPostStore) getParentsPostsPostgreSQL(channelId string, offset int, limit int, skipFetchThreads bool) ([]*model.Post, *model.AppError) {
	var posts []*model.Post
	replyCountQuery := ""
	onStatement := "q1.RootId = q2.Id"
	if skipFetchThreads {
		replyCountQuery = ` ,(SELECT COUNT(Posts.Id) FROM Posts WHERE q2.RootId = '' AND Posts.RootId = q2.Id AND Posts.DeleteAt = 0) as ReplyCount`
	} else {
		onStatement += " OR q1.RootId = q2.RootId"
	}
	_, err := s.GetReplica().Select(&posts,
		`SELECT q2.*`+replyCountQuery+`
        FROM
            Posts q2
                INNER JOIN
            (SELECT DISTINCT
                q3.RootId
            FROM
                (SELECT
                    RootId
                FROM
                    Posts
                WHERE
                    ChannelId = :ChannelId1
                        AND DeleteAt = 0
                ORDER BY CreateAt DESC
                LIMIT :Limit OFFSET :Offset) q3
            WHERE q3.RootId != '') q1
            ON `+onStatement+`
        WHERE
            ChannelId = :ChannelId2
                AND DeleteAt = 0
        ORDER BY CreateAt`,
		map[string]interface{}{"ChannelId1": channelId, "Offset": offset, "Limit": limit, "ChannelId2": channelId})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetLinearPosts", "store.sql_post.get_parents_posts.app_error", nil, "channelId="+channelId+" err="+err.Error(), http.StatusInternalServerError)
	}
	return posts, nil
}

var specialSearchChar = []string{
	"<",
	">",
	"+",
	"-",
	"(",
	")",
	"~",
	"@",
	":",
}

func (s *SqlPostStore) buildCreateDateFilterClause(params *model.SearchParams, queryParams map[string]interface{}) (string, map[string]interface{}) {
	searchQuery := ""
	// handle after: before: on: filters
	if len(params.OnDate) > 0 {
		onDateStart, onDateEnd := params.GetOnDateMillis()
		queryParams["OnDateStart"] = strconv.FormatInt(onDateStart, 10)
		queryParams["OnDateEnd"] = strconv.FormatInt(onDateEnd, 10)

		// between `on date` start of day and end of day
		searchQuery += "AND CreateAt BETWEEN :OnDateStart AND :OnDateEnd "
	} else {

		if len(params.ExcludedDate) > 0 {
			excludedDateStart, excludedDateEnd := params.GetExcludedDateMillis()
			queryParams["ExcludedDateStart"] = strconv.FormatInt(excludedDateStart, 10)
			queryParams["ExcludedDateEnd"] = strconv.FormatInt(excludedDateEnd, 10)

			searchQuery += "AND CreateAt NOT BETWEEN :ExcludedDateStart AND :ExcludedDateEnd "
		}

		if len(params.AfterDate) > 0 {
			afterDate := params.GetAfterDateMillis()
			queryParams["AfterDate"] = strconv.FormatInt(afterDate, 10)

			// greater than `after date`
			searchQuery += "AND CreateAt >= :AfterDate "
		}

		if len(params.BeforeDate) > 0 {
			beforeDate := params.GetBeforeDateMillis()
			queryParams["BeforeDate"] = strconv.FormatInt(beforeDate, 10)

			// less than `before date`
			searchQuery += "AND CreateAt <= :BeforeDate "
		}

		if len(params.ExcludedAfterDate) > 0 {
			afterDate := params.GetExcludedAfterDateMillis()
			queryParams["ExcludedAfterDate"] = strconv.FormatInt(afterDate, 10)

			searchQuery += "AND CreateAt < :ExcludedAfterDate "
		}

		if len(params.ExcludedBeforeDate) > 0 {
			beforeDate := params.GetExcludedBeforeDateMillis()
			queryParams["ExcludedBeforeDate"] = strconv.FormatInt(beforeDate, 10)

			searchQuery += "AND CreateAt > :ExcludedBeforeDate "
		}
	}

	return searchQuery, queryParams
}

func (s *SqlPostStore) buildSearchChannelFilterClause(channels []string, paramPrefix string, exclusion bool, queryParams map[string]interface{}) (string, map[string]interface{}) {
	if len(channels) == 0 {
		return "", queryParams
	}

	clauseSlice := []string{}
	for i, channel := range channels {
		paramName := paramPrefix + strconv.FormatInt(int64(i), 10)
		clauseSlice = append(clauseSlice, ":"+paramName)
		queryParams[paramName] = channel
	}
	clause := strings.Join(clauseSlice, ", ")
	if exclusion {
		return "AND Name NOT IN (" + clause + ")", queryParams
	}
	return "AND Name IN (" + clause + ")", queryParams
}

func (s *SqlPostStore) buildSearchUserFilterClause(users []string, paramPrefix string, exclusion bool, queryParams map[string]interface{}) (string, map[string]interface{}) {
	if len(users) == 0 {
		return "", queryParams
	}
	clauseSlice := []string{}
	for i, user := range users {
		paramName := paramPrefix + strconv.FormatInt(int64(i), 10)
		clauseSlice = append(clauseSlice, ":"+paramName)
		queryParams[paramName] = user
	}
	clause := strings.Join(clauseSlice, ", ")
	if exclusion {
		return "AND Username NOT IN (" + clause + ")", queryParams
	}
	return "AND Username IN (" + clause + ")", queryParams
}

func (s *SqlPostStore) buildSearchPostFilterClause(fromUsers []string, excludedUsers []string, queryParams map[string]interface{}) (string, map[string]interface{}) {
	if len(fromUsers) == 0 && len(excludedUsers) == 0 {
		return "", queryParams
	}

	filterQuery := `
		AND UserId IN (
			SELECT
				Id
			FROM
				Users,
				TeamMembers
			WHERE
				TeamMembers.TeamId = :TeamId
				AND Users.Id = TeamMembers.UserId
				FROM_USER_FILTER
				EXCLUDED_USER_FILTER)`

	fromUserClause, queryParams := s.buildSearchUserFilterClause(fromUsers, "FromUser", false, queryParams)
	filterQuery = strings.Replace(filterQuery, "FROM_USER_FILTER", fromUserClause, 1)

	excludedUserClause, queryParams := s.buildSearchUserFilterClause(excludedUsers, "ExcludedUser", true, queryParams)
	filterQuery = strings.Replace(filterQuery, "EXCLUDED_USER_FILTER", excludedUserClause, 1)

	return filterQuery, queryParams
}

func (s *SqlPostStore) Search(teamId string, userId string, params *model.SearchParams) (*model.PostList, *model.AppError) {
	queryParams := map[string]interface{}{
		"TeamId": teamId,
		"UserId": userId,
	}

	list := model.NewPostList()
	if params.Terms == "" && params.ExcludedTerms == "" &&
		len(params.InChannels) == 0 && len(params.ExcludedChannels) == 0 &&
		len(params.FromUsers) == 0 && len(params.ExcludedUsers) == 0 &&
		len(params.OnDate) == 0 && len(params.AfterDate) == 0 && len(params.BeforeDate) == 0 {
		return list, nil
	}

	var posts []*model.Post

	deletedQueryPart := "AND DeleteAt = 0"
	if params.IncludeDeletedChannels {
		deletedQueryPart = ""
	}

	userIdPart := "AND UserId = :UserId"
	if params.SearchWithoutUserId {
		userIdPart = ""
	}

	searchQuery := `
			SELECT
				* ,(SELECT COUNT(Posts.Id) FROM Posts WHERE q2.RootId = '' AND Posts.RootId = q2.Id AND Posts.DeleteAt = 0) as ReplyCount
			FROM
				Posts q2
			WHERE
				DeleteAt = 0
				AND Type NOT LIKE '` + model.POST_SYSTEM_MESSAGE_PREFIX + `%'
				POST_FILTER
				AND ChannelId IN (
					SELECT
						Id
					FROM
						Channels,
						ChannelMembers
					WHERE
						Id = ChannelId
							AND (TeamId = :TeamId OR TeamId = '')
							` + userIdPart + `
							` + deletedQueryPart + `
							IN_CHANNEL_FILTER
							EXCLUDED_CHANNEL_FILTER)
				CREATEDATE_CLAUSE
				SEARCH_CLAUSE
				ORDER BY CreateAt DESC
			LIMIT 100`

	inChannelClause, queryParams := s.buildSearchChannelFilterClause(params.InChannels, "InChannel", false, queryParams)
	searchQuery = strings.Replace(searchQuery, "IN_CHANNEL_FILTER", inChannelClause, 1)

	excludedChannelClause, queryParams := s.buildSearchChannelFilterClause(params.ExcludedChannels, "ExcludedChannel", true, queryParams)
	searchQuery = strings.Replace(searchQuery, "EXCLUDED_CHANNEL_FILTER", excludedChannelClause, 1)

	postFilterClause, queryParams := s.buildSearchPostFilterClause(params.FromUsers, params.ExcludedUsers, queryParams)
	searchQuery = strings.Replace(searchQuery, "POST_FILTER", postFilterClause, 1)

	createDateFilterClause, queryParams := s.buildCreateDateFilterClause(params, queryParams)
	searchQuery = strings.Replace(searchQuery, "CREATEDATE_CLAUSE", createDateFilterClause, 1)

	termMap := map[string]bool{}
	terms := params.Terms
	excludedTerms := params.ExcludedTerms

	searchType := "Message"
	if params.IsHashtag {
		searchType = "Hashtags"
		for _, term := range strings.Split(terms, " ") {
			termMap[strings.ToUpper(term)] = true
		}
	}

	// these chars have special meaning and can be treated as spaces
	for _, c := range specialSearchChar {
		terms = strings.Replace(terms, c, " ", -1)
		excludedTerms = strings.Replace(excludedTerms, c, " ", -1)
	}

	if terms == "" && excludedTerms == "" {
		// we've already confirmed that we have a channel or user to search for
		searchQuery = strings.Replace(searchQuery, "SEARCH_CLAUSE", "", 1)
	} else if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		// Parse text for wildcards
		if wildcard, err := regexp.Compile(`\*($| )`); err == nil {
			terms = wildcard.ReplaceAllLiteralString(terms, ":* ")
			excludedTerms = wildcard.ReplaceAllLiteralString(excludedTerms, ":* ")
		}

		excludeClause := ""
		if excludedTerms != "" {
			excludeClause = " & !(" + strings.Join(strings.Fields(excludedTerms), " | ") + ")"
		}

		if params.OrTerms {
			queryParams["Terms"] = "(" + strings.Join(strings.Fields(terms), " | ") + ")" + excludeClause
		} else {
			queryParams["Terms"] = "(" + strings.Join(strings.Fields(terms), " & ") + ")" + excludeClause
		}

		searchClause := fmt.Sprintf("AND to_tsvector('english', %s) @@  to_tsquery('english', :Terms)", searchType)
		searchQuery = strings.Replace(searchQuery, "SEARCH_CLAUSE", searchClause, 1)
	} else if s.DriverName() == model.DATABASE_DRIVER_MYSQL {
		searchClause := fmt.Sprintf("AND MATCH (%s) AGAINST (:Terms IN BOOLEAN MODE)", searchType)
		searchQuery = strings.Replace(searchQuery, "SEARCH_CLAUSE", searchClause, 1)

		excludeClause := ""
		if excludedTerms != "" {
			excludeClause = " -(" + excludedTerms + ")"
		}

		if params.OrTerms {
			queryParams["Terms"] = terms + excludeClause
		} else {
			splitTerms := []string{}
			for _, t := range strings.Fields(terms) {
				splitTerms = append(splitTerms, "+"+t)
			}
			queryParams["Terms"] = strings.Join(splitTerms, " ") + excludeClause
		}
	}

	_, err := s.GetSearchReplica().Select(&posts, searchQuery, queryParams)
	if err != nil {
		mlog.Warn("Query error searching posts.", mlog.Err(err))
		// Don't return the error to the caller as it is of no use to the user. Instead return an empty set of search results.
	} else {
		for _, p := range posts {
			if searchType == "Hashtags" {
				exactMatch := false
				for _, tag := range strings.Split(p.Hashtags, " ") {
					if termMap[strings.ToUpper(tag)] {
						exactMatch = true
						break
					}
				}
				if !exactMatch {
					continue
				}
			}
			list.AddPost(p)
			list.AddOrder(p.Id)
		}
	}
	list.MakeNonNil()
	return list, nil
}

func (s *SqlPostStore) AnalyticsUserCountsWithPostsByDay(teamId string) (model.AnalyticsRows, *model.AppError) {
	query :=
		`SELECT DISTINCT
		        DATE(FROM_UNIXTIME(Posts.CreateAt / 1000)) AS Name,
		        COUNT(DISTINCT Posts.UserId) AS Value
		FROM Posts`

	if len(teamId) > 0 {
		query += " INNER JOIN Channels ON Posts.ChannelId = Channels.Id AND Channels.TeamId = :TeamId AND"
	} else {
		query += " WHERE"
	}

	query += ` Posts.CreateAt >= :StartTime AND Posts.CreateAt <= :EndTime
		GROUP BY DATE(FROM_UNIXTIME(Posts.CreateAt / 1000))
		ORDER BY Name DESC
		LIMIT 30`

	if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		query =
			`SELECT
				TO_CHAR(DATE(TO_TIMESTAMP(Posts.CreateAt / 1000)), 'YYYY-MM-DD') AS Name, COUNT(DISTINCT Posts.UserId) AS Value
			FROM Posts`

		if len(teamId) > 0 {
			query += " INNER JOIN Channels ON Posts.ChannelId = Channels.Id AND Channels.TeamId = :TeamId AND"
		} else {
			query += " WHERE"
		}

		query += ` Posts.CreateAt >= :StartTime AND Posts.CreateAt <= :EndTime
			GROUP BY DATE(TO_TIMESTAMP(Posts.CreateAt / 1000))
			ORDER BY Name DESC
			LIMIT 30`
	}

	end := utils.MillisFromTime(utils.EndOfDay(utils.Yesterday()))
	start := utils.MillisFromTime(utils.StartOfDay(utils.Yesterday().AddDate(0, 0, -31)))

	var rows model.AnalyticsRows
	_, err := s.GetReplica().Select(
		&rows,
		query,
		map[string]interface{}{"TeamId": teamId, "StartTime": start, "EndTime": end})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.AnalyticsUserCountsWithPostsByDay", "store.sql_post.analytics_user_counts_posts_by_day.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	return rows, nil
}

func (s *SqlPostStore) AnalyticsPostCountsByDay(options *model.AnalyticsPostCountsOptions) (model.AnalyticsRows, *model.AppError) {

	query :=
		`SELECT
		        DATE(FROM_UNIXTIME(Posts.CreateAt / 1000)) AS Name,
		        COUNT(Posts.Id) AS Value
		    FROM Posts`

	if options.BotsOnly {
		query += " INNER JOIN Bots ON Posts.UserId = Bots.Userid"
	}

	if len(options.TeamId) > 0 {
		query += " INNER JOIN Channels ON Posts.ChannelId = Channels.Id AND Channels.TeamId = :TeamId AND"
	} else {
		query += " WHERE"
	}

	query += ` Posts.CreateAt <= :EndTime
		            AND Posts.CreateAt >= :StartTime
		GROUP BY DATE(FROM_UNIXTIME(Posts.CreateAt / 1000))
		ORDER BY Name DESC
		LIMIT 30`

	if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		query =
			`SELECT
				TO_CHAR(DATE(TO_TIMESTAMP(Posts.CreateAt / 1000)), 'YYYY-MM-DD') AS Name, Count(Posts.Id) AS Value
			FROM Posts`

		if options.BotsOnly {
			query += " INNER JOIN Bots ON Posts.UserId = Bots.Userid"
		}

		if len(options.TeamId) > 0 {
			query += " INNER JOIN Channels ON Posts.ChannelId = Channels.Id  AND Channels.TeamId = :TeamId AND"
		} else {
			query += " WHERE"
		}

		query += ` Posts.CreateAt <= :EndTime
			            AND Posts.CreateAt >= :StartTime
			GROUP BY DATE(TO_TIMESTAMP(Posts.CreateAt / 1000))
			ORDER BY Name DESC
			LIMIT 30`
	}

	end := utils.MillisFromTime(utils.EndOfDay(utils.Yesterday()))
	start := utils.MillisFromTime(utils.StartOfDay(utils.Yesterday().AddDate(0, 0, -31)))
	if options.YesterdayOnly {
		start = utils.MillisFromTime(utils.StartOfDay(utils.Yesterday().AddDate(0, 0, -1)))
	}

	var rows model.AnalyticsRows
	_, err := s.GetReplica().Select(
		&rows,
		query,
		map[string]interface{}{"TeamId": options.TeamId, "StartTime": start, "EndTime": end})
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.AnalyticsPostCountsByDay", "store.sql_post.analytics_posts_count_by_day.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	return rows, nil
}

func (s *SqlPostStore) AnalyticsPostCount(teamId string, mustHaveFile bool, mustHaveHashtag bool) (int64, *model.AppError) {
	query :=
		`SELECT
			COUNT(Posts.Id) AS Value
		FROM
			Posts,
			Channels
		WHERE
			Posts.ChannelId = Channels.Id`

	if len(teamId) > 0 {
		query += " AND Channels.TeamId = :TeamId"
	}

	if mustHaveFile {
		query += " AND (Posts.FileIds != '[]' OR Posts.Filenames != '[]')"
	}

	if mustHaveHashtag {
		query += " AND Posts.Hashtags != ''"
	}

	v, err := s.GetReplica().SelectInt(query, map[string]interface{}{"TeamId": teamId})
	if err != nil {
		return 0, model.NewAppError("SqlPostStore.AnalyticsPostCount", "store.sql_post.analytics_posts_count.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	return v, nil
}

func (s *SqlPostStore) GetPostsCreatedAt(channelId string, time int64) ([]*model.Post, *model.AppError) {
	query := `SELECT * FROM Posts WHERE CreateAt = :CreateAt AND ChannelId = :ChannelId`

	var posts []*model.Post
	_, err := s.GetReplica().Select(&posts, query, map[string]interface{}{"CreateAt": time, "ChannelId": channelId})

	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPostsCreatedAt", "store.sql_post.get_posts_created_att.app_error", nil, "channelId="+channelId+err.Error(), http.StatusInternalServerError)
	}
	return posts, nil
}

func (s *SqlPostStore) GetPostsByIds(postIds []string) ([]*model.Post, *model.AppError) {
	keys, params := MapStringsToQueryParams(postIds, "Post")

	query := `SELECT * FROM Posts WHERE Id IN ` + keys + ` ORDER BY CreateAt DESC`

	var posts []*model.Post
	_, err := s.GetReplica().Select(&posts, query, params)

	if err != nil {
		mlog.Error("Query error getting posts.", mlog.Err(err))
		return nil, model.NewAppError("SqlPostStore.GetPostsByIds", "store.sql_post.get_posts_by_ids.app_error", nil, "", http.StatusInternalServerError)
	}
	return posts, nil
}

func (s *SqlPostStore) GetPostsBatchForIndexing(startTime int64, endTime int64, limit int) ([]*model.PostForIndexing, *model.AppError) {
	var posts []*model.PostForIndexing
	_, err := s.GetSearchReplica().Select(&posts,
		`SELECT
			PostsQuery.*, Channels.TeamId, ParentPosts.CreateAt ParentCreateAt
		FROM (
			SELECT
				*
			FROM
				Posts
			WHERE
				Posts.CreateAt >= :StartTime
			AND
				Posts.CreateAt < :EndTime
			ORDER BY
				CreateAt ASC
			LIMIT
				1000
			)
		AS
			PostsQuery
		LEFT JOIN
			Channels
		ON
			PostsQuery.ChannelId = Channels.Id
		LEFT JOIN
			Posts ParentPosts
		ON
			PostsQuery.RootId = ParentPosts.Id`,
		map[string]interface{}{"StartTime": startTime, "EndTime": endTime, "NumPosts": limit})

	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetPostContext", "store.sql_post.get_posts_batch_for_indexing.get.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	return posts, nil
}

func (s *SqlPostStore) PermanentDeleteBatch(endTime int64, limit int64) (int64, *model.AppError) {
	var query string
	if s.DriverName() == "postgres" {
		query = "DELETE from Posts WHERE Id = any (array (SELECT Id FROM Posts WHERE CreateAt < :EndTime LIMIT :Limit))"
	} else {
		query = "DELETE from Posts WHERE CreateAt < :EndTime LIMIT :Limit"
	}

	sqlResult, err := s.GetMaster().Exec(query, map[string]interface{}{"EndTime": endTime, "Limit": limit})
	if err != nil {
		return 0, model.NewAppError("SqlPostStore.PermanentDeleteBatch", "store.sql_post.permanent_delete_batch.app_error", nil, ""+err.Error(), http.StatusInternalServerError)
	}

	rowsAffected, err := sqlResult.RowsAffected()
	if err != nil {
		return 0, model.NewAppError("SqlPostStore.PermanentDeleteBatch", "store.sql_post.permanent_delete_batch.app_error", nil, ""+err.Error(), http.StatusInternalServerError)
	}
	return rowsAffected, nil
}

func (s *SqlPostStore) GetOldest() (*model.Post, *model.AppError) {
	var post model.Post
	err := s.GetReplica().SelectOne(&post, "SELECT * FROM Posts ORDER BY CreateAt LIMIT 1")
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetOldest", "store.sql_post.get.app_error", nil, err.Error(), http.StatusNotFound)
	}

	return &post, nil
}

func (s *SqlPostStore) determineMaxPostSize() int {
	var maxPostSizeBytes int32

	if s.DriverName() == model.DATABASE_DRIVER_POSTGRES {
		// The Post.Message column in Postgres has historically been VARCHAR(4000), but
		// may be manually enlarged to support longer posts.
		if err := s.GetReplica().SelectOne(&maxPostSizeBytes, `
			SELECT
				COALESCE(character_maximum_length, 0)
			FROM
				information_schema.columns
			WHERE
				table_name = 'posts'
			AND	column_name = 'message'
		`); err != nil {
			mlog.Error("Unable to determine the maximum supported post size", mlog.Err(err))
		}
	} else if s.DriverName() == model.DATABASE_DRIVER_MYSQL {
		// The Post.Message column in MySQL has historically been TEXT, with a maximum
		// limit of 65535.
		if err := s.GetReplica().SelectOne(&maxPostSizeBytes, `
			SELECT
				COALESCE(CHARACTER_MAXIMUM_LENGTH, 0)
			FROM
				INFORMATION_SCHEMA.COLUMNS
			WHERE
				table_schema = DATABASE()
			AND	table_name = 'Posts'
			AND	column_name = 'Message'
			LIMIT 0, 1
		`); err != nil {
			mlog.Error("Unable to determine the maximum supported post size", mlog.Err(err))
		}
	} else {
		mlog.Warn("No implementation found to determine the maximum supported post size")
	}

	// Assume a worst-case representation of four bytes per rune.
	maxPostSize := int(maxPostSizeBytes) / 4

	// To maintain backwards compatibility, don't yield a maximum post
	// size smaller than the previous limit, even though it wasn't
	// actually possible to store 4000 runes in all cases.
	if maxPostSize < model.POST_MESSAGE_MAX_RUNES_V1 {
		maxPostSize = model.POST_MESSAGE_MAX_RUNES_V1
	}

	mlog.Info("Post.Message has size restrictions", mlog.Int("max_characters", maxPostSize), mlog.Int32("max_bytes", maxPostSizeBytes))

	return maxPostSize
}

// GetMaxPostSize returns the maximum number of runes that may be stored in a post.
func (s *SqlPostStore) GetMaxPostSize() int {
	s.maxPostSizeOnce.Do(func() {
		s.maxPostSizeCached = s.determineMaxPostSize()
	})
	return s.maxPostSizeCached
}

func (s *SqlPostStore) GetParentsForExportAfter(limit int, afterId string) ([]*model.PostForExport, *model.AppError) {
	for {
		var rootIds []string
		_, err := s.GetReplica().Select(&rootIds,
			`SELECT
				Id
			FROM
				Posts
			WHERE
				Id > :AfterId
				AND RootId = ''
				AND DeleteAt = 0
			ORDER BY Id
			LIMIT :Limit`,
			map[string]interface{}{"Limit": limit, "AfterId": afterId})
		if err != nil {
			return nil, model.NewAppError("SqlPostStore.GetAllAfterForExport", "store.sql_post.get_posts.app_error",
				nil, err.Error(), http.StatusInternalServerError)
		}

		var postsForExport []*model.PostForExport
		if len(rootIds) == 0 {
			return postsForExport, nil
		}

		keys, params := MapStringsToQueryParams(rootIds, "PostId")
		_, err = s.GetSearchReplica().Select(&postsForExport, `
			SELECT
				p1.*,
				Users.Username as Username,
				Teams.Name as TeamName,
				Channels.Name as ChannelName
			FROM
				(Select * FROM Posts WHERE Id IN `+keys+`) p1
			INNER JOIN
				Channels ON p1.ChannelId = Channels.Id
			INNER JOIN
				Teams ON Channels.TeamId = Teams.Id
			INNER JOIN
				Users ON p1.UserId = Users.Id
			WHERE
				Channels.DeleteAt = 0
				AND Teams.DeleteAt = 0
			ORDER BY
				p1.Id`,
			params)
		if err != nil {
			return nil, model.NewAppError("SqlPostStore.GetAllAfterForExport", "store.sql_post.get_posts.app_error",
				nil, err.Error(), http.StatusInternalServerError)
		}

		if len(postsForExport) == 0 {
			// All of the posts were in channels or teams that were deleted.
			// Update the afterId and try again.
			afterId = rootIds[len(rootIds)-1]
			continue
		}

		return postsForExport, nil
	}
}

func (s *SqlPostStore) GetRepliesForExport(rootId string) ([]*model.ReplyForExport, *model.AppError) {
	var posts []*model.ReplyForExport
	_, err := s.GetSearchReplica().Select(&posts, `
			SELECT
				Posts.*,
				Users.Username as Username
			FROM
				Posts
			INNER JOIN
				Users ON Posts.UserId = Users.Id
			WHERE
				Posts.RootId = :RootId
				AND Posts.DeleteAt = 0
			ORDER BY
				Posts.Id`,
		map[string]interface{}{"RootId": rootId})

	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetAllAfterForExport", "store.sql_post.get_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	return posts, nil
}

func (s *SqlPostStore) GetDirectPostParentsForExportAfter(limit int, afterId string) ([]*model.DirectPostForExport, *model.AppError) {
	query := s.getQueryBuilder().
		Select("p.*", "Users.Username as User").
		From("Posts p").
		Join("Channels ON p.ChannelId = Channels.Id").
		Join("Users ON p.UserId = Users.Id").
		Where(sq.And{
			sq.Gt{"p.Id": afterId},
			sq.Eq{"p.ParentId": string("")},
			sq.Eq{"p.DeleteAt": int(0)},
			sq.Eq{"Channels.DeleteAt": int(0)},
			sq.Eq{"Users.DeleteAt": int(0)},
			sq.Eq{"Channels.Type": []string{"D", "G"}},
		}).
		OrderBy("p.Id").
		Limit(uint64(limit))

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetDirectPostParentsForExportAfter", "store.sql_post.get_direct_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	var posts []*model.DirectPostForExport
	if _, err = s.GetReplica().Select(&posts, queryString, args...); err != nil {
		return nil, model.NewAppError("SqlPostStore.GetDirectPostParentsForExportAfter", "store.sql_post.get_direct_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	var channelIds []string
	for _, post := range posts {
		channelIds = append(channelIds, post.ChannelId)
	}
	query = s.getQueryBuilder().
		Select("u.Username as Username, ChannelId, UserId, cm.Roles as Roles, LastViewedAt, MsgCount, MentionCount, cm.NotifyProps as NotifyProps, LastUpdateAt, SchemeUser, SchemeAdmin, (SchemeGuest IS NOT NULL AND SchemeGuest) as SchemeGuest").
		From("ChannelMembers cm").
		Join("Users u ON ( u.Id = cm.UserId )").
		Where(sq.Eq{
			"cm.ChannelId": channelIds,
		})

	queryString, args, err = query.ToSql()
	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetDirectPostParentsForExportAfter", "store.sql_post.get_direct_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	var channelMembers []*model.ChannelMemberForExport
	if _, err := s.GetReplica().Select(&channelMembers, queryString, args...); err != nil {
		return nil, model.NewAppError("SqlPostStore.GetDirectPostParentsForExportAfter", "store.sql_post.get_direct_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// Build a map of channels and their posts
	postsChannelMap := make(map[string][]*model.DirectPostForExport)
	for _, post := range posts {
		post.ChannelMembers = &[]string{}
		postsChannelMap[post.ChannelId] = append(postsChannelMap[post.ChannelId], post)
	}

	// Build a map of channels and their members
	channelMembersMap := make(map[string][]string)
	for _, member := range channelMembers {
		channelMembersMap[member.ChannelId] = append(channelMembersMap[member.ChannelId], member.Username)
	}

	// Populate each post ChannelMembers extracting it from the channelMembersMap
	for channelId := range channelMembersMap {
		for _, post := range postsChannelMap[channelId] {
			*post.ChannelMembers = channelMembersMap[channelId]
		}
	}
	return posts, nil
}

// GetRecentPosts () returns at most perChannel recent posts for given channel ids.
//
func (s *SqlPostStore) GetRecentPosts(channelIds *[]string, perChannel int) (*[]model.Post, *model.AppError) {
	var posts []model.Post
	_, err := s.GetReplica().Select(&posts, `
			WITH c AS (
				SELECT Id AS ChannelId,LastPostAt 
				FROM Channels 
				WHERE Id in ('`+strings.Join(*channelIds, "','")+`')
			)
			SELECT p.* 
			FROM c 
			JOIN LATERAL (
				SELECT * 
				FROM Posts as p 
				WHERE p.channelid = c.channelid 
					AND p.createat <= c.lastpostat 
					AND p.deleteat = 0
					ORDER BY p.createat DESC 
					LIMIT :Limit
			) AS p ON true
			`,
		map[string]interface{}{"Limit": perChannel},
	)

	if err != nil {
		return nil, model.NewAppError("SqlPostStore.GetRecentPosts", "store.sql_post.get_recent_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	return &posts, nil
}

// GetPostsCountAfter () sums the number of posts since the given post id for all of the given channels.
//
func (s *SqlPostStore) GetPostCountAfter(channels *[]model.ChannelWithPost) (int64, *model.AppError) {
	if counts, err := s.GetPostCountAfterForChannels(channels); err != nil {
		return 0, err
	} else {
		var res int64 = 0
		for _, v := range *counts {
			res += int64(v)
		}
		return res, nil
	}
}

// GetPostsCountAfter () calculates the number of posts since the given post id for all of the given channels.
//
func (s *SqlPostStore) GetPostCountAfterForChannels(channels *[]model.ChannelWithPost) (*map[string]int, *model.AppError) {
	postIds, badReq := s.getValidPostIdsForChannels(
		channels,
		"SqlPostStore.GetPostCountAfterForChannels",
		"get_posts_count_after_for_channels",
	)
	if badReq != nil {
		return nil, badReq
	}

	var counts []struct {
		ChannelId string
		Count     int
	}
	_, err := s.GetReplica().Select(&counts,
		`
		WITH PostDate AS (
			SELECT ChannelId,CreateAt 
			FROM Posts
			WHERE Id in ('`+strings.Join(*postIds, "','")+`')
			ORDER BY ChannelId
		)
		SELECT ChannelId, count 
		FROM
			(SELECT ChannelId, ChannelCount.count
			FROM PostDate 
			JOIN LATERAL (
				SELECT count(*) 
				FROM 
					(SELECT 1 FROM posts
					WHERE channelid = PostDate.channelid 
						AND createat > PostDate.createat 
						AND deleteat = 0
						LIMIT 1000) as p
			) AS ChannelCount ON true) as PostChannelCount		
		`,
	)

	if err != nil {
		return nil, model.NewAppError(
			"SqlPostStore.GetPostCountAfterForChannels",
			"store.sql_post.get_posts_count_after.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
	}

	result := make(map[string]int, len(counts))
	for _, v := range counts {
		result[v.ChannelId] = v.Count
	}
	return &result, nil
}

// GetTotalPosts () return a sum of total numbers of posts in all given channels
//
func (s *SqlPostStore) GetTotalPosts(channelIds *[]string) (int64, *model.AppError) {
	count, err := s.GetReplica().SelectInt(
		`
		SELECT sum(totalmsgcount)
		FROM Channels
		WHERE id IN ('` + strings.Join(*channelIds, "','") + `')
		`,
	)
	if err != nil {
		return 0, model.NewAppError(
			"SqlPostStore.GetTotalPostsCount",
			"store.sql_post.get_total_posts_count.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
	}
	return count, nil
}

// GetTotalPostsForChannels () returns total number of posts for all given channels.
//
func (s *SqlPostStore) GetTotalPostsForChannels(channelIds *[]string) (*map[string]int, *model.AppError) {
	var counts []struct {
		ChannelId string
		Count     int
	}
	_, err := s.GetReplica().Select(&counts,
		`
		SELECT Id as ChannelId, TotalMsgCount as Count
		FROM Channels
		WHERE id IN ('`+strings.Join(*channelIds, "','")+`')
		`,
	)
	if err != nil {
		return nil, model.NewAppError(
			"SqlPostStore.GetTotalPostsForChannels",
			"store.sql_post.get_total_posts_count_for_channels.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
	}
	result := make(map[string]int, len(counts))
	for _, v := range counts {
		result[v.ChannelId] = v.Count
	}
	return &result, nil
}

// GetOldestPostsForChannels () returns the id of the oldest post for all given channels.
//
func (s *SqlPostStore) GetOldestPostsForChannels(channelIds *[]string) (*map[string]string, *model.AppError) {
	oldestPosts := []model.ChannelWithPost{}
	_, err := s.GetReplica().Select(&oldestPosts,
		`
		WITH c AS (
			SELECT Id as ChannelId
			FROM Channels
			WHERE Id IN ('`+strings.Join(*channelIds, "','")+`')
			ORDER BY ChannelId
		)
		SELECT c.ChannelId, p.*
		FROM c
		JOIN LATERAL (
			SELECT p.Id as PostId
			FROM Posts as p 
			WHERE p.ChannelId = c.ChannelId
				AND p.DeleteAt = 0
				ORDER BY p.createat ASC 
				LIMIT 1
		) AS p ON true	
		`,
	)
	if err != nil {
		return nil, model.NewAppError(
			"SqlPostStore.GetOldestPostsForChannels",
			"store.sql_post.get_oldest_posts_channels.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
	}
	result := make(map[string]string, len(oldestPosts))
	for _, v := range *channelIds {
		result[v] = ""
	}
	for _, v := range oldestPosts {
		result[v.ChannelId] = v.PostId
	}
	return &result, nil
}

func (s *SqlPostStore) getValidPostIdsForChannels(
	channels *[]model.ChannelWithPost,
	where string,
	errorId string,
) (*[]string, *model.AppError) {
	channelByPost := make(map[string]string, len(*channels))
	postIds := make([]string, len(*channels))
	for i, v := range *channels {
		postIds[i] = v.PostId
		channelByPost[v.PostId] = v.ChannelId
	}

	// Get channel ids and creation time for all given posts, distinct on channel id
	var requestData []struct {
		ChannelId string
		PostId    string
		CreateAt  int64
	}
	_, err := s.GetReplica().Select(&requestData, `
		SELECT DISTINCT ON (ChannelId) ChannelId,Id as PostId,CreateAt 
		FROM Posts
		WHERE Id IN ('`+strings.Join(postIds, "','")+`')
		`,
	)

	if err != nil {
		return nil, model.NewAppError(
			where,
			"store.sql_post."+errorId+".failed_to_get_posts.app_error",
			nil,
			"Duplicate or missing channels in requested post list",
			http.StatusInternalServerError,
		)
	}

	// Check that number of channels pulled from db equals the number of channels requested
	if len(requestData) != len(*channels) {
		return nil, model.NewAppError(
			where,
			"store.sql_post."+errorId+".channel_id_duplicate_or_missing.app_error",
			nil,
			"Duplicate or missing channels in requested post list",
			http.StatusInternalServerError,
		)
	}

	// Check that given posts really belong to given channels
	for _, v := range requestData {
		if cid, exists := channelByPost[v.PostId]; exists {
			if cid != v.ChannelId {
				return nil, model.NewAppError(
					where,
					"store.sql_post."+errorId+".channel_id_post_id_mismatch.app_error",
					nil,
					"Unexpected channel id for post",
					http.StatusInternalServerError,
				)
			}
		}
	}

	return &postIds, nil
}

// GetAllPostsAfter () returns all post between a given post id and the most recent post
// for every given channel.
//
// includeIds parameter lists post ids that are to be included in the result.
// This is necessary when requesting posts from the very beginning of a channel,
// otherwise we would return everything except the first post.
//
// postCountsByChannel parameter helps to allocate correct amount of memory
// for the resulting lists, so they don't get extended and reallocated when
// the map is created.
// NB: zero posts found for a channel will cause a gorp error because
// this would yield a row of null values in join.
//
func (s *SqlPostStore) GetAllPostsAfter(
	channels *[]model.ChannelWithPost,
	includeIds *[]string,
	postCountsByChannel *map[string]int,
) (*map[string]*[]*model.Post, *model.AppError) {
	postIds, reqErr := s.getValidPostIdsForChannels(
		channels,
		"SqlPostStore.GetAllPostsAfter",
		"get_all_posts_after",
	)
	if reqErr != nil {
		return nil, reqErr
	}

	var include string
	if len(*includeIds) > 0 {
		include = "OR p.Id IN ('" + strings.Join(*includeIds, "','") + "')"
	}
	var posts []model.Post
	_, err := s.GetReplica().Select(&posts,
		`
		WITH PostDate AS (
			SELECT ChannelId,CreateAt 
			FROM Posts
			WHERE Id IN ('`+strings.Join(*postIds, "','")+`')
			ORDER BY ChannelId
		)
		SELECT p.*
		FROM PostDate
		JOIN LATERAL (
			SELECT *
			FROM Posts as p 
			WHERE p.ChannelId = PostDate.ChannelId
				AND p.DeleteAt = 0
				AND (
					p.CreateAt > PostDate.CreateAt 
					`+include+`
				)
				ORDER BY p.CreateAt ASC 
				LIMIT :Limit
		) AS p ON true	
		`,
		map[string]interface{}{"Limit": 1000},
	)

	if err != nil {
		return nil, model.NewAppError(
			"SqlPostStore.GetAllPostsAfter",
			"store.sql_post.get_all_posts_after.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
	}

	// Copy result pointers to a map by channel id.
	// Create lists for all channel ids even if no posts were found.
	result := map[string]*[]*model.Post{}
	for _, v := range *channels {
		var l []*model.Post
		if c, exists := (*postCountsByChannel)[v.ChannelId]; exists {
			l = make([]*model.Post, c)[:0]
		} else {
			l = []*model.Post{}
		}
		result[v.ChannelId] = &l
	}
	for _, v := range posts {
		p := v
		list := result[v.ChannelId]
		*list = append(*list, &p)
	}
	return &result, nil
}

type PostBrief struct {
	Id        string
	ChannelId string
}

func (s *SqlPostStore) CheckForUpdates(userId string, list *[]model.ChannelWithPost) (*model.ChannelUpdates, *model.AppError) {
	channelIds := make([]string, len(*list))
	channelById := make(map[string]*model.ChannelWithPost, len(*list))
	for i, v := range *list {
		channelIds[i] = v.ChannelId
		channelById[v.ChannelId] = &((*list)[i])
	}
	sort.Strings(channelIds)
	idsParam := "'" + strings.Join(channelIds, "','") + "'"

	// Which channels the user is a member of?
	var existingIds []string
	_, err := s.GetMaster().Select(
		&existingIds,
		`
		SELECT cm.ChannelId FROM channelmembers as cm,channels as c
		WHERE 
			cm.UserId = :UserId AND cm.channelid = c.id AND c.DeleteAt = 0
		ORDER BY ChannelId
		`,
		map[string]interface{}{"UserId": userId},
	)
	if err != nil {
		return nil, model.NewAppError(
			"CheckForUpdates",
			"post_store.check_for_update",
			nil,
			fmt.Sprintf("Failed to select existing channel memberships: %s", err.Error()),
			http.StatusInternalServerError,
		)
	}

	// Which channels among the requested are deleted?
	var deleted []string
	_, err = s.GetReplica().Select(
		&deleted,
		`
		SELECT Id FROM channels 
		WHERE 
			DeleteAt != 0 
			AND Id IN (`+idsParam+`)
		ORDER BY Id
		`,
	)
	if err != nil {
		return nil, model.NewAppError(
			"CheckForUpdates",
			"post_store.check_for_update",
			nil,
			fmt.Sprintf("Failed to select deleted channels: %s", err.Error()),
			http.StatusInternalServerError,
		)
	}

	// Remove deleted from current membership
	memberCheck := make(map[string]bool, len(existingIds))
	lastIndex := 0
	for _, v := range existingIds {
		found := false
		for j := lastIndex; j < len(deleted); j++ {
			if deleted[j] == v {
				found = true
				lastIndex = j
				break
			}
		}
		if !found {
			memberCheck[v] = true
		}
	}

	// Which channels among the requested the user is no longer a member of?
	removed := []string{}
	for _, v := range channelIds {
		if _, ok := memberCheck[v]; !ok {
			removed = append(removed, v)
		}
	}

	// Which channels NOT among the requested the user is a member of?
	added := []string{}
	toRequestUpdates := make([]string, 0, len(channelIds))
	for id := range memberCheck {
		if _, ok := channelById[id]; !ok {
			added = append(added, id)
		} else {
			toRequestUpdates = append(toRequestUpdates, id)
		}
	}

	// Which of the existing memberships have more messages?
	// Get last messages for channels
	var posts []PostBrief
	_, err = s.GetReplica().Select(&posts,
		`
		SELECT
			p.Id, p.ChannelId
		FROM
			Posts as p
		JOIN (
			SELECT Id, LastPostAt FROM Channels
			WHERE Id IN ('`+strings.Join(toRequestUpdates, "','")+`')
		) as c
		ON p.ChannelId=c.Id and c.LastPostAt=p.CreateAt
		`,
	)
	if err != nil {
		return nil, model.NewAppError(
			"CheckForUpdates",
			"post_store.check_for_update",
			nil,
			fmt.Sprintf("Failed to select last posts: %s", err.Error()),
			http.StatusInternalServerError,
		)
	}
	updated := make([]string, 0, len(toRequestUpdates))
	for _, v := range posts {
		r := channelById[v.ChannelId]
		if r.PostId != v.Id {
			updated = append(updated, r.ChannelId)
		}
	}

	result := model.ChannelUpdates{
		Added:   &added,
		Removed: &removed,
		Updated: &updated,
	}
	return &result, nil
}
